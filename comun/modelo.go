package comun

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"sync"
)

func Sigmoide(z float64) float64 { return 1 / (1 + math.Exp(-z)) }

type parcial struct {
	indice  int
	grad    []float64
	perdida float64
}

// gradiente de las filas [ini, fin); solo lee X, Y y w
func gradienteParcial(X [][]float64, Y, w []float64, ini, fin, indice int, salida chan<- parcial, wg *sync.WaitGroup) {
	defer wg.Done()
	grad := make([]float64, len(w))
	perdida := 0.0
	for i := ini; i < fin; i++ {
		x := X[i]
		z := 0.0
		for j := range w {
			z += w[j] * x[j]
		}
		p := Sigmoide(z)
		e := p - Y[i]
		for j := range w {
			grad[j] += e * x[j]
		}
		p = math.Min(math.Max(p, 1e-12), 1-1e-12)
		perdida += -(Y[i]*math.Log(p) + (1-Y[i])*math.Log(1-p))
	}
	salida <- parcial{indice, grad, perdida}
}

// descenso de gradiente con W workers por iteracion; lote = 0 usa todo el conjunto
func Entrenar(d *Datos, workers, epocas int, tasa float64, lote int, informe func(epoca int, perdida float64)) ([]float64, []float64) {
	n := len(d.X)
	dim := len(d.Nombres)
	w := make([]float64, dim)
	if lote <= 0 || lote > n {
		lote = n
	}
	perdidas := make([]float64, 0, epocas)
	suma := make([]float64, dim)
	for e := 0; e < epocas; e++ {
		perdidaEpoca := 0.0
		for ini := 0; ini < n; ini += lote {
			fin := ini + lote
			if fin > n {
				fin = n
			}
			parciales := make(chan parcial, workers)
			var wg sync.WaitGroup
			tam := (fin - ini + workers - 1) / workers
			for k := 0; k < workers; k++ {
				a := ini + k*tam
				b := a + tam
				if a > fin {
					a = fin
				}
				if b > fin {
					b = fin
				}
				wg.Add(1)
				go gradienteParcial(d.X, d.Y, w, a, b, k, parciales, &wg)
			}
			go func() { wg.Wait(); close(parciales) }()

			grads := make([][]float64, workers)
			for p := range parciales {
				grads[p.indice] = p.grad
				perdidaEpoca += p.perdida
			}
			for j := range suma {
				suma[j] = 0
			}
			for k := 0; k < workers; k++ { // orden fijo para que el resultado sea el mismo con cualquier W
				for j := range suma {
					suma[j] += grads[k][j]
				}
			}
			m := float64(fin - ini)
			for j := range w {
				w[j] -= tasa * suma[j] / m
			}
		}
		perdidaEpoca /= float64(n)
		perdidas = append(perdidas, perdidaEpoca)
		if informe != nil {
			informe(e+1, perdidaEpoca)
		}
	}
	return w, perdidas
}

func Predecir(X [][]float64, w []float64, workers int) []float64 {
	n := len(X)
	p := make([]float64, n)
	var wg sync.WaitGroup
	tam := (n + workers - 1) / workers
	for k := 0; k < workers; k++ {
		a, b := k*tam, (k+1)*tam
		if a > n {
			a = n
		}
		if b > n {
			b = n
		}
		wg.Add(1)
		go func(a, b int) {
			defer wg.Done()
			for i := a; i < b; i++ {
				z := 0.0
				for j := range w {
					z += w[j] * X[i][j]
				}
				p[i] = Sigmoide(z)
			}
		}(a, b)
	}
	wg.Wait()
	return p
}

type Metricas struct {
	Umbral    float64 `json:"umbral"`
	Accuracy  float64 `json:"accuracy"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	AUC       float64 `json:"auc"`
	VP        int     `json:"vp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	VN        int     `json:"vn"`
}

func Evaluar(y, p []float64, umbral float64) Metricas {
	m := Metricas{Umbral: umbral}
	for i := range y {
		pred := p[i] >= umbral
		switch {
		case pred && y[i] == 1:
			m.VP++
		case pred && y[i] == 0:
			m.FP++
		case !pred && y[i] == 1:
			m.FN++
		default:
			m.VN++
		}
	}
	total := float64(len(y))
	m.Accuracy = float64(m.VP+m.VN) / total
	if m.VP+m.FP > 0 {
		m.Precision = float64(m.VP) / float64(m.VP+m.FP)
	}
	if m.VP+m.FN > 0 {
		m.Recall = float64(m.VP) / float64(m.VP+m.FN)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	m.AUC = AUC(y, p)
	return m
}

// AUC por rangos (Mann-Whitney)
func AUC(y, p []float64) float64 {
	n := len(y)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return p[idx[a]] < p[idx[b]] })
	sumaRangosPos := 0.0
	nPos, nNeg := 0.0, 0.0
	for i := 0; i < n; {
		j := i
		for j < n && p[idx[j]] == p[idx[i]] {
			j++
		}
		rango := float64(i+1+j) / 2
		for k := i; k < j; k++ {
			if y[idx[k]] == 1 {
				sumaRangosPos += rango
				nPos++
			} else {
				nNeg++
			}
		}
		i = j
	}
	if nPos == 0 || nNeg == 0 {
		return 0
	}
	return (sumaRangosPos - nPos*(nPos+1)/2) / (nPos * nNeg)
}

func MejorUmbral(y, p []float64) Metricas {
	mejor := Evaluar(y, p, 0.5)
	for u := 0.05; u <= 0.95; u += 0.01 {
		m := Evaluar(y, p, u)
		if m.F1 > mejor.F1 {
			mejor = m
		}
	}
	return mejor
}

type Modelo struct {
	Columnas     []string           `json:"columnas"`
	Pesos        []float64          `json:"pesos"`
	OddsRatios   map[string]float64 `json:"odds_ratios"`
	Codebook     *Codebook          `json:"codebook"`
	Workers      int                `json:"workers"`
	Epocas       int                `json:"epocas"`
	Tasa         float64            `json:"tasa"`
	Lote         int                `json:"lote"`
	FilasTrain   int                `json:"filas_entrenamiento"`
	FilasTest    int                `json:"filas_prueba"`
	PerdidaFinal float64            `json:"perdida_final"`
	Metricas05   Metricas           `json:"metricas_umbral_0.5"`
	MetricasF1   Metricas           `json:"metricas_mejor_f1"`
}

func GuardarModelo(ruta string, m Modelo) error {
	m.OddsRatios = map[string]float64{}
	for j, nombre := range m.Columnas {
		if nombre != "sesgo" {
			m.OddsRatios[nombre] = math.Exp(m.Pesos[j])
		}
	}
	contenido, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, contenido, 0644)
}
