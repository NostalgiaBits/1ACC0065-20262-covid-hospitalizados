package comun

import (
	"math"
	"testing"
)

func datosPrueba(n int) *Datos {
	d := &Datos{Nombres: []string{"sesgo", "x"}}
	for i := 0; i < n; i++ {
		x := float64(i%21-10) / 10 // -1 ... 1
		d.X = append(d.X, []float64{1, x})
		if x > 0 {
			d.Y = append(d.Y, 1)
		} else {
			d.Y = append(d.Y, 0)
		}
	}
	return d
}

// una iteracion: gradiente paralelo = gradiente a mano
func TestGradienteUnaIteracion(t *testing.T) {
	d := datosPrueba(50)
	esperado := make([]float64, 2)
	for i := range d.X {
		for j := range esperado {
			esperado[j] += (0.5 - d.Y[i]) * d.X[i][j]
		}
	}
	tasa := 1.0
	w, _ := Entrenar(d, 4, 1, tasa, 0, nil)
	for j := range w {
		quiero := -tasa * esperado[j] / float64(len(d.X))
		if math.Abs(w[j]-quiero) > 1e-12 {
			t.Errorf("peso %d: %v, esperado %v", j, w[j], quiero)
		}
	}
}

// mismo resultado con 1 y con varios workers
func TestEntrenarDeterminista(t *testing.T) {
	d := datosPrueba(1000)
	w1, p1 := Entrenar(d, 1, 50, 0.5, 0, nil)
	for _, workers := range []int{2, 7, 24} {
		wk, pk := Entrenar(d, workers, 50, 0.5, 0, nil)
		for j := range w1 {
			if math.Abs(w1[j]-wk[j]) > 1e-9 {
				t.Errorf("workers %d: peso %d difiere: %v vs %v", workers, j, w1[j], wk[j])
			}
		}
		if math.Abs(p1[len(p1)-1]-pk[len(pk)-1]) > 1e-9 {
			t.Errorf("workers %d: perdida final difiere", workers)
		}
	}
	if p1[len(p1)-1] >= p1[0] {
		t.Errorf("la perdida no baja: %v -> %v", p1[0], p1[len(p1)-1])
	}
}

func TestMiniLoteAprende(t *testing.T) {
	d := datosPrueba(1000)
	w, _ := Entrenar(d, 3, 5, 0.5, 100, nil)
	p := Predecir(d.X, w, 3)
	m := Evaluar(d.Y, p, 0.5)
	if m.Accuracy < 0.95 {
		t.Errorf("accuracy baja con mini-lotes: %v", m.Accuracy)
	}
}

func TestAUC(t *testing.T) {
	y := []float64{0, 0, 1, 1}
	if a := AUC(y, []float64{0.1, 0.4, 0.35, 0.8}); math.Abs(a-0.75) > 1e-12 {
		t.Errorf("AUC = %v, esperado 0.75", a)
	}
	if a := AUC(y, []float64{0.1, 0.2, 0.3, 0.4}); a != 1 {
		t.Errorf("AUC perfecta = %v", a)
	}
	if a := AUC(y, []float64{0.5, 0.5, 0.5, 0.5}); math.Abs(a-0.5) > 1e-12 {
		t.Errorf("AUC con empates = %v, esperado 0.5", a)
	}
}

func TestEvaluar(t *testing.T) {
	m := Evaluar([]float64{1, 1, 0, 0, 1}, []float64{0.9, 0.2, 0.7, 0.1, 0.6}, 0.5)
	if m.VP != 2 || m.FN != 1 || m.FP != 1 || m.VN != 1 {
		t.Errorf("matriz: %+v", m)
	}
	if math.Abs(m.Precision-2.0/3) > 1e-12 || math.Abs(m.Recall-2.0/3) > 1e-12 || math.Abs(m.Accuracy-0.6) > 1e-12 {
		t.Errorf("metricas: %+v", m)
	}
}
