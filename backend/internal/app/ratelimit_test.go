package app

import (
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

	// Fallos: con 3 permitidos, el tercero ya llega al límite.
	if !l.Fail("user", 3, time.Minute) || !l.Fail("user", 3, time.Minute) || l.Blocked("user", 3, time.Minute) {
		t.Fatal("dos fallos no deben bloquear")
	}
	if l.Fail("user", 3, time.Minute) || !l.Blocked("user", 3, time.Minute) {
		t.Fatal("el tercer fallo debe bloquear")
	}
	l.Reset("user")
	if l.Blocked("user", 3, time.Minute) {
		t.Fatal("Reset debe desbloquear")
	}

	// La ventana es deslizante: el primer fallo vence antes que los demás.
	l.Fail("k", 2, time.Minute)
	now = now.Add(40 * time.Second)
	l.Fail("k", 2, time.Minute)
	if !l.Blocked("k", 2, time.Minute) {
		t.Fatal("dos fallos en la ventana deben bloquear")
	}
	now = now.Add(21 * time.Second)
	if l.Blocked("k", 2, time.Minute) {
		t.Fatal("el primer fallo ya salió de la ventana")
	}

	now = now.Add(time.Hour)
	l.cleanup()
	if len(l.hits) != 0 {
		t.Fatalf("cleanup dejó %d claves", len(l.hits))
	}
}
