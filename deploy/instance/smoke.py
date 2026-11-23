#!/usr/bin/env python3
"""Prueba de despliegue con la biblioteca estándar, sin modificar datos.

SMOKE_BASE_URL=http://localhost:8080 python3 deploy/instance/smoke.py
SMOKE_REQUIRE_WEB=1 también revisa la SPA y sus archivos JS/CSS.
SMOKE_ADMIN_PASSWORD permite verificar acceso, consultas y salida del administrador.
SMOKE_PUBLIC_URL indica el Origin de CSRF al probar el dominio de Caddy.
SMOKE_CONNECT_HOST=127.0.0.1 conserva SNI y la validación del certificado.
SMOKE_ORIGIN_SECRET añade X-Origin-Verify; ningún secreto se imprime.
"""
import http.client
import json
import os
import re
import socket
import ssl
import sys
from http.cookies import SimpleCookie
from urllib.parse import urlsplit


BASE = urlsplit(os.environ.get('SMOKE_BASE_URL', 'http://localhost:8080'))
if BASE.scheme not in ('http', 'https') or not BASE.hostname or BASE.path.rstrip('/'):
    raise SystemExit('SMOKE_BASE_URL must be an http(s) origin without a path.')
PUBLIC = os.environ.get('SMOKE_PUBLIC_URL', f'{BASE.scheme}://{BASE.netloc}')
CONNECT_HOST = os.environ.get('SMOKE_CONNECT_HOST')
SECRET = os.environ.get('SMOKE_ORIGIN_SECRET')
REQUIRE_WEB = os.environ.get('SMOKE_REQUIRE_WEB') == '1'
COOKIE = ''
PASSED = 0


class LocalTLSConnection(http.client.HTTPSConnection):
    def connect(self):
        target = CONNECT_HOST or self.host
        self.sock = socket.create_connection((target, self.port), self.timeout)
        self.sock = self._context.wrap_socket(self.sock, server_hostname=self.host)


def require(ok, message):
    if not ok:
        raise AssertionError(message)


def get(path, method='GET', payload=None, auth=False, expected=200, json_body=True):
    global PASSED
    headers = {'Accept': 'application/json', 'User-Agent': 'diseno-deployment-smoke/1'}
    if SECRET:
        headers['X-Origin-Verify'] = SECRET
    if auth and COOKIE:
        headers['Cookie'] = COOKIE
    if method != 'GET':
        headers['Content-Type'] = 'application/json'
        headers['Origin'] = PUBLIC
    data = None if payload is None else json.dumps(payload).encode()
    if BASE.scheme == 'https':
        connection = LocalTLSConnection(BASE.hostname, BASE.port or 443, timeout=15,
                                        context=ssl.create_default_context())
    else:
        connection = http.client.HTTPConnection(CONNECT_HOST or BASE.hostname,
                                                BASE.port or 80, timeout=15)
        if CONNECT_HOST:
            headers['Host'] = BASE.netloc
    try:
        connection.request(method, path, body=data, headers=headers)
        response = connection.getresponse()
        raw = response.read(5 * 1024 * 1024)
        response_headers = response.headers
        require(response.status == expected,
                f'{method} {path}: expected {expected}, got {response.status}; '
                'check Caddy/WAF/origin settings if the response is a challenge.')
        require(response_headers.get('X-Content-Type-Options') == 'nosniff',
                f'{path}: missing nosniff')
        require(response_headers.get('X-Frame-Options') == 'DENY',
                f'{path}: missing frame protection')
        if json_body:
            require('application/json' in response_headers.get('Content-Type', ''),
                    f'{path}: response is not JSON')
            body = json.loads(raw)
            require('no-store' in response_headers.get('Cache-Control', ''),
                    f'{path}: API response should not be cached')
        else:
            body = raw
        PASSED += 1
        print(f'PASS {method} {path}')
        return body, response_headers
    finally:
        connection.close()


def main():
    global COOKIE
    health, _ = get('/api/health')
    require(health.get('ok') is True, 'health is not healthy')
    config, _ = get('/api/config')
    require(isinstance(config.get('courses'), list), 'config.courses must be an array')
    require(isinstance(config.get('levels'), list), 'config.levels must be an array')
    require(isinstance(config.get('features'), dict), 'config.features is missing')
    ids = [course['id'] for course in config['courses']]
    require(len(ids) == len(set(ids)), 'public course ids are not unique')
    for flag in ('registration', 'passwordReset', 'store', 'leaderboard', 'levelUpAnimation'):
        require(isinstance(config['features'].get(flag), bool), f'feature {flag} is not boolean')
    get('/api/admin/stats', expected=401)
    get('/api/does-not-exist', expected=404)
    get('/api/me', expected=401)
    if REQUIRE_WEB:
        page, headers = get('/', json_body=False)
        require('text/html' in headers.get('Content-Type', ''), 'homepage is not HTML')
        require('no-cache' in headers.get('Cache-Control', ''), 'HTML should revalidate')
        require(b'id="root"' in page, 'React root is missing')
        assets = sorted(set(re.findall(rb'(?:src|href)="(/assets/[^"?#]+)', page)))
        require(bool(assets), 'compiled Vite assets are missing')
        for asset in assets:
            _, asset_headers = get(asset.decode(), json_body=False)
            require('immutable' in asset_headers.get('Cache-Control', ''),
                    'Vite asset should have immutable caching')
        get('/admin', json_body=False)
        get('/admin/alumnos', json_body=False)
        get('/assets/does-not-exist.js', expected=404, json_body=False)
    password = os.environ.get('SMOKE_ADMIN_PASSWORD')
    if password:
        _, headers = get('/api/admin-login', 'POST', {
            'username': os.environ.get('SMOKE_ADMIN_USERNAME', 'Admin'),
            'password': password,
        })
        cookies = SimpleCookie()
        for value in headers.get_all('Set-Cookie', []):
            cookies.load(value)
        require('diseno_admin' in cookies, 'administrator session cookie missing')
        session = cookies['diseno_admin']
        require(bool(session['httponly']), 'administrator cookie must be HttpOnly')
        require(session['samesite'].lower() == 'lax', 'administrator cookie must be SameSite=Lax')
        if BASE.scheme == 'https':
            require(bool(session['secure']), 'HTTPS administrator cookie must be Secure')
        COOKIE = f'diseno_admin={session.value}'
        try:
            account, _ = get('/api/admin/me', auth=True)
            require(isinstance(account.get('admin'), dict), 'administrator session is invalid')
            get('/api/admin/stats', auth=True)
            for path, key in (
                    ('/api/admin/students', 'students'), ('/api/admin/courses', 'courses'),
                    ('/api/admin/badges', 'badges'), ('/api/admin/levels', 'levels'),
                    ('/api/admin/avatars', 'avatars'), ('/api/admin/purchases', 'purchases'),
                    ('/api/admin/admins', 'admins')):
                body, _ = get(path, auth=True)
                require(isinstance(body.get(key), list), f'{path}: {key} is not an array')
            get('/api/admin/settings', auth=True)
            get('/api/admin/import/status', auth=True)
            get('/api/admin/audit?limit=5', auth=True)
        finally:
            get('/api/admin/logout', 'POST', {}, auth=True)
            COOKIE = ''
        get('/api/admin/me', auth=True, expected=401)
    print(f'PASS {PASSED} deployment checks; no business data was changed.')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Los errores muestran rutas y comprobaciones, nunca cuerpos, cookies o claves.
        print(f'FAIL {type(error).__name__}: {error}', file=sys.stderr)
        sys.exit(1)
