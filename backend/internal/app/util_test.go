package app

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// Los esperados salen de correr las mismas expresiones del código viejo en JavaScript.

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Kali Call Práctico":    "kali_call_practico",
		"  Internet  Explorer ": "_internet_explorer_",
		"Ñandú":                 "nandu",
		"Calculadora!":          "calculadora",
		"Día-de muertos":        "diade_muertos",
		"Tab\tSep":              "tab_sep",
		"Ünïcödé 2":             "unicode_2",
		"Café\u00a0Bar":         "cafe_bar",
		"":                      "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

func TestNormalizeLabel(t *testing.T) {
	cases := map[string]string{
		"Análisis_de-Datos ":           "analisis de datos",
		"  Calculadora  Científica ":   "calculadora cientifica",
		"INTERNET":                     "internet",
		"a__--b":                       "a b",
		"Técnicas Avanzadas\u00a0de R": "tecnicas avanzadas de r",
		"":                             "",
	}
	for in, want := range cases {
		if got := normalizeLabel(in); got != want {
			t.Errorf("normalizeLabel(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

func TestParseIntLoose(t *testing.T) {
	cases := map[string]int{
		"12abc":         12,
		"":              0,
		" -7 ":          -7,
		"1,000":         1,
		"abc":           0,
		"+5":            5,
		"-":             0,
		"+-5":           0,
		"3.9":           3,
		"0x1A":          0,
		"\u00a0\t42":    42,
		"007":           7,
		"99999999999":   math.MaxInt32,
		"-99999999999":  -math.MaxInt32,
		"１２":            0, // dígitos de ancho completo: JS da NaN
		"\ufeff8 coins": 8,
	}
	for in, want := range cases {
		if got := parseIntLoose(in); got != want {
			t.Errorf("parseIntLoose(%q) = %d, esperaba %d", in, got, want)
		}
	}
}

func TestRandomDigits(t *testing.T) {
	d := randomDigits(6)
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(d) {
		t.Fatalf("randomDigits(6) = %q", d)
	}
}

func TestGeneratePassword(t *testing.T) {
	for range 200 {
		pw, err := generatePassword("^[A-Z0-9]{4}$")
		if err != nil {
			t.Fatal(err)
		}
		var nl, nd int
		for _, r := range pw {
			switch {
			case r >= 'A' && r <= 'Z':
				nl++
			case r >= '0' && r <= '9':
				nd++
			}
		}
		if len(pw) != 4 || nl != 2 || nd != 2 || strings.ContainsAny(pw, "IO01") {
			t.Fatalf("contraseña generada inválida: %q", pw)
		}
	}
	if _, err := generatePassword("^[a-z]{6}$"); err == nil {
		t.Fatal("un patrón incompatible debió dar error")
	}
	if _, err := generatePassword("("); err == nil {
		t.Fatal("un patrón inválido debió dar error")
	}
}
