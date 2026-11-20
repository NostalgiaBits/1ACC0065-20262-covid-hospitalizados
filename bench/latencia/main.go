// Prueba de carga de /predecir: N peticiones con C clientes concurrentes.
// Uso: go run ./bench/latencia [url] [peticiones] [concurrencia]
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

func main() {
	url, total, conc := "http://localhost:8080/predecir", 2000, 20
	if len(os.Args) > 1 {
		url = os.Args[1]
	}
	if len(os.Args) > 2 {
		total, _ = strconv.Atoi(os.Args[2])
	}
	if len(os.Args) > 3 {
		conc, _ = strconv.Atoi(os.Args[3])
	}

	deps := []string{"LIMA", "AREQUIPA", "CALLAO", "CUSCO", "PIURA", "LAMBAYEQUE", "JUNIN", "ICA", "LORETO", "PUNO"}
	periodos := []string{"2020-S1", "2020-S2", "2021-S1", "2021-S2", "2022+"}
	fabs := []string{"NINGUNA", "PFIZER", "SINOPHARM", "ASTRAZENECA"}
	r := rand.New(rand.NewSource(42))
	cuerpos := make([][]byte, total)
	for i := range cuerpos {
		dosis := r.Intn(4)
		fab := fabs[0]
		if dosis > 0 {
			fab = fabs[1+r.Intn(3)]
		}
		p := map[string]any{"edad": 20 + r.Intn(70), "sexo": []string{"F", "M"}[r.Intn(2)], "dosis_previas": dosis,
			"fabricante": fab, "departamento": deps[r.Intn(len(deps))], "periodo": periodos[r.Intn(len(periodos))]}
		cuerpos[i], _ = json.Marshal(p)
	}

	cliente := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: conc}}
	tiempos := make([]float64, total)
	servidor := make([]float64, total)
	errores, hits := 0, 0
	var mu sync.Mutex
	trabajos := make(chan int, total)
	for i := 0; i < total; i++ {
		trabajos <- i
	}
	close(trabajos)

	inicio := time.Now()
	var wg sync.WaitGroup
	for k := 0; k < conc; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range trabajos {
				t0 := time.Now()
				resp, err := cliente.Post(url, "application/json", bytes.NewReader(cuerpos[i]))
				ms := float64(time.Since(t0).Microseconds()) / 1000
				var res struct {
					Cache      string  `json:"cache"`
					LatenciaMs float64 `json:"latencia_ms"`
				}
				if err == nil {
					json.NewDecoder(resp.Body).Decode(&res)
					resp.Body.Close()
				}
				mu.Lock()
				tiempos[i], servidor[i] = ms, res.LatenciaMs
				if err != nil || resp.StatusCode != 200 {
					errores++
				} else if res.Cache == "hit" {
					hits++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	dur := time.Since(inicio).Seconds()

	sort.Float64s(tiempos)
	sort.Float64s(servidor)
	p := func(v []float64, q float64) float64 { return v[int(q/100*float64(len(v)-1))] }
	bajo := sort.SearchFloat64s(tiempos, 100)
	fmt.Printf("Peticiones: %d · concurrencia %d · errores %d · cache hits %d · %.0f peticiones/s\n", total, conc, errores, hits, float64(total)/dur)
	fmt.Printf("Tiempo de respuesta (cliente): p50 %.2f ms · p95 %.2f ms · p99 %.2f ms · max %.2f ms\n", p(tiempos, 50), p(tiempos, 95), p(tiempos, 99), tiempos[len(tiempos)-1])
	fmt.Printf("Tiempo en el servidor:         p50 %.3f ms · p95 %.3f ms · p99 %.3f ms · max %.3f ms\n", p(servidor, 50), p(servidor, 95), p(servidor, 99), servidor[len(servidor)-1])
	fmt.Printf("Respuestas bajo 100 ms: %.2f %%\n", 100*float64(bajo)/float64(total))
}
