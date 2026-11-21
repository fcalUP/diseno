package app

import (
	"crypto/rand"
	"errors"
	"math"
	"math/big"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// isJSSpace replica \s de JavaScript (y lo que quita String.prototype.trim).
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00a0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// collapseRuns cambia cada racha de caracteres que cumplen match por repl.
func collapseRuns(s string, match func(rune) bool, repl string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if match(r) {
			if !in {
				b.WriteString(repl)
			}
			in = true
			continue
		}
		in = false
		b.WriteRune(r)
	}
	return b.String()
}

// normalizeLabel es el normalizeLabel del código viejo: NFD, quita acentos
// (U+0300–U+036F), minúsculas, [_-]+ → espacio, colapsa espacios y recorta.
func normalizeLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 0x0300 && r <= 0x036f {
			return -1
		}
		return r
	}, norm.NFD.String(s))
	s = strings.ToLower(s)
	s = collapseRuns(s, func(r rune) bool { return r == '_' || r == '-' }, " ")
	s = collapseRuns(s, isJSSpace, " ")
	return strings.TrimFunc(s, isJSSpace)
}

// slugify es el nombre de archivo de insignia del código viejo: minúsculas,
// espacios → "_", NFD y solo [a-zA-Z0-9_]. "Kali Call Práctico" → "kali_call_practico".
func slugify(name string) string {
	s := collapseRuns(strings.ToLower(name), isJSSpace, "_")
	return strings.Map(func(r rune) rune {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, norm.NFD.String(s))
}

// parseIntLoose imita parseInt(x, 10) de JavaScript; NaN → 0. Como las columnas
// son integer, lo que no cabe en int32 se satura.
func parseIntLoose(s string) int {
	s = strings.TrimLeftFunc(s, isJSSpace)
	sign := 1
	if s != "" && (s[0] == '-' || s[0] == '+') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		if n <= math.MaxInt32 {
			n = n*10 + int(s[i]-'0')
		}
	}
	return sign * min(n, math.MaxInt32)
}

// cryptoIntn da un entero al azar en [0, n) usando crypto/rand.
func cryptoIntn(n int) int {
	v, _ := rand.Int(rand.Reader, big.NewInt(int64(n)))
	return int(v.Int64())
}

// randomDigits devuelve n dígitos al azar (crypto/rand).
func randomDigits(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('0' + cryptoIntn(10))
	}
	return string(b)
}

// Sin I, O, 0 ni 1 para que no se confundan al copiarlas de una lista impresa.
const (
	passwordLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	passwordDigits  = "23456789"
)

// generatePassword arma 2 letras mayúsculas + 2 dígitos en orden aleatorio y
// exige que cumpla pattern (la regla de contraseña de Ajustes).
func generatePassword(pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	for range 50 {
		pw := []byte{
			passwordLetters[cryptoIntn(len(passwordLetters))],
			passwordLetters[cryptoIntn(len(passwordLetters))],
			passwordDigits[cryptoIntn(len(passwordDigits))],
			passwordDigits[cryptoIntn(len(passwordDigits))],
		}
		for i := len(pw) - 1; i > 0; i-- {
			j := cryptoIntn(i + 1)
			pw[i], pw[j] = pw[j], pw[i]
		}
		if re.MatchString(string(pw)) {
			return string(pw), nil
		}
	}
	return "", errors.New("la regla de contraseña no admite contraseñas generadas (2 letras y 2 números); captúrala a mano")
}
