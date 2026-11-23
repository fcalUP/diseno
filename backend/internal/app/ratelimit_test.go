package app

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	now := time.Now()
	l := &Limiter{hits: map[string]*hitLog{}, now: func() time.Time { return now }}

	for i := range 3 {
		if !l.Allow("ip", 3, time.Minute) {
			t.Fatalf("intento %d debió pasar", i+1)
		}
	}
	if l.Allow("ip", 3, time.Minute) {
		t.Fatal("el cuarto intento debió bloquearse")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("ip", 3, time.Minute) {
		t.Fatal("al salir de la ventana debió pasar")
	}

	l.Allow("user", 2, time.Minute)
	l.Allow("user", 2, time.Minute)
	l.Reset("user")
	if !l.Allow("user", 2, time.Minute) {
		t.Fatal("Reset debe desbloquear")
	}

	// La ventana es deslizante: el primer intento vence antes que los demás.
	l.Allow("k", 2, time.Minute)
	now = now.Add(40 * time.Second)
	l.Allow("k", 2, time.Minute)
	if l.Allow("k", 2, time.Minute) {
		t.Fatal("dos intentos en la ventana deben bloquear")
	}
	now = now.Add(21 * time.Second)
	if !l.Allow("k", 2, time.Minute) {
		t.Fatal("el primer intento ya salió de la ventana")
	}

	now = now.Add(time.Hour)
	l.cleanup()
	if len(l.hits) != 0 {
		t.Fatalf("cleanup dejó %d claves", len(l.hits))
	}
}

// Regresión (H1): revisar y contar en un solo paso. Con 50 a la vez solo pasan max.
func TestLimiterAllowConcurrent(t *testing.T) {
	l := &Limiter{hits: map[string]*hitLog{}, now: time.Now}
	var mu sync.Mutex
	var wg sync.WaitGroup
	passed := 0
	for range 50 {
		wg.Go(func() {
			if l.Allow("k", 10, time.Minute) {
				mu.Lock()
				passed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if passed != 10 {
		t.Fatalf("pasaron %d intentos, esperaba 10", passed)
	}
}
