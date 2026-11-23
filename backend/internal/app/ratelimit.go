package app

import (
	"sync"
	"time"
)

// Limiter cuenta eventos por clave en una ventana deslizante, en memoria.
// Una clave se usa siempre con la misma ventana (p. ej. "login-ip:1.2.3.4").
// Regla común: con count eventos en la ventana, se permite seguir si count < max.
type Limiter struct {
	mu   sync.Mutex
	hits map[string]*hitLog
	now  func() time.Time // reemplazable en pruebas
}

type hitLog struct {
	times  []time.Time // del más viejo al más nuevo
	window time.Duration
}

func NewLimiter() *Limiter {
	l := &Limiter{hits: map[string]*hitLog{}, now: time.Now}
	go func() {
		for range time.Tick(time.Minute) {
			l.cleanup()
		}
	}()
	return l
}

// Allow cuenta un intento; false (sin contarlo) si ya hay max en la ventana.
// Revisar y contar van juntos: un login lo llama antes de bcrypt, así una
// ráfaga en paralelo no pasa completa antes de que se anote el primer fallo.
func (l *Limiter) Allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.count(key, window, now) >= max {
		return false
	}
	l.add(key, max, window, now)
	return true
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}

// count descarta lo que salió de la ventana y devuelve cuántos quedan.
func (l *Limiter) count(key string, window time.Duration, now time.Time) int {
	h := l.hits[key]
	if h == nil {
		return 0
	}
	h.window = window
	cut := now.Add(-window)
	i := 0
	for i < len(h.times) && !h.times[i].After(cut) {
		i++
	}
	h.times = h.times[i:]
	return len(h.times)
}

func (l *Limiter) add(key string, max int, window time.Duration, now time.Time) {
	l.count(key, window, now)
	h := l.hits[key]
	if h == nil {
		h = &hitLog{window: window}
		l.hits[key] = h
	}
	h.times = append(h.times, now)
	if len(h.times) > max { // solo importan los últimos max
		h.times = h.times[len(h.times)-max:]
	}
}

func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for k, h := range l.hits {
		if len(h.times) == 0 || now.Sub(h.times[len(h.times)-1]) > h.window {
			delete(l.hits, k)
		}
	}
}
