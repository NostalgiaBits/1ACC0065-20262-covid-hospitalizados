package main

import (
	"math"
	"sort"
	"sync"
	"time"
)

// latencias de /predecir en memoria (ultimas 1000)
type Metricas struct {
	mu             sync.Mutex
	inicio         time.Time
	latencias      []float64
	pos            int
	predicciones   int64
	cacheHits      int64
	errores        int64
	entrenamientos int64
}

func NuevasMetricas() *Metricas {
	return &Metricas{inicio: time.Now(), latencias: make([]float64, 0, 1000)}
}

func (m *Metricas) Registrar(ms float64, hit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.latencias) < 1000 {
		m.latencias = append(m.latencias, ms)
	} else {
		m.latencias[m.pos] = ms
		m.pos = (m.pos + 1) % 1000
	}
	m.predicciones++
	if hit {
		m.cacheHits++
	}
}

func (m *Metricas) Error() {
	m.mu.Lock()
	m.errores++
	m.mu.Unlock()
}

func (m *Metricas) Entrenamiento() {
	m.mu.Lock()
	m.entrenamientos++
	m.mu.Unlock()
}

func percentil(ordenadas []float64, p float64) float64 {
	if len(ordenadas) == 0 {
		return 0
	}
	i := int(math.Ceil(p/100*float64(len(ordenadas)))) - 1
	if i < 0 {
		i = 0
	}
	return ordenadas[i]
}

func (m *Metricas) Resumen() map[string]any {
	m.mu.Lock()
	copia := append([]float64(nil), m.latencias...)
	res := map[string]any{
		"predicciones": m.predicciones, "cache_hits": m.cacheHits, "errores_validacion": m.errores,
		"entrenamientos": m.entrenamientos, "activo_desde": m.inicio.Format(time.RFC3339),
	}
	m.mu.Unlock()
	sort.Float64s(copia)
	suma := 0.0
	for _, v := range copia {
		suma += v
	}
	lat := map[string]any{"muestras": len(copia)}
	if len(copia) > 0 {
		lat["media_ms"] = redondear(suma / float64(len(copia)))
		lat["p50_ms"] = redondear(percentil(copia, 50))
		lat["p95_ms"] = redondear(percentil(copia, 95))
		lat["p99_ms"] = redondear(percentil(copia, 99))
		lat["max_ms"] = redondear(copia[len(copia)-1])
		bajo100 := sort.SearchFloat64s(copia, 100)
		lat["bajo_100ms_pct"] = redondear(100 * float64(bajo100) / float64(len(copia)))
	}
	res["latencia_predecir"] = lat
	return res
}

func redondear(x float64) float64 { return math.Round(x*1000) / 1000 }
