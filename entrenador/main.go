// Uso: go run ./entrenador <train.csv> <test.csv> <workers> <epocas> <tasa> [lote] [modelo.json]
package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"covid-hospitalizados/comun"
)

func main() {
	if len(os.Args) < 6 {
		fmt.Println("Uso: entrenador <train.csv> <test.csv> <workers> <epocas> <tasa> [lote=0] [modelo.json]")
		os.Exit(1)
	}
	rutaTrain, rutaTest := os.Args[1], os.Args[2]
	workers, err1 := strconv.Atoi(os.Args[3])
	epocas, err2 := strconv.Atoi(os.Args[4])
	tasa, err3 := strconv.ParseFloat(os.Args[5], 64)
	if err1 != nil || err2 != nil || err3 != nil || workers < 1 || epocas < 1 || tasa <= 0 {
		fmt.Println("workers y epocas deben ser enteros >= 1; tasa un numero > 0")
		os.Exit(1)
	}
	lote := 0
	if len(os.Args) > 6 {
		lote, _ = strconv.Atoi(os.Args[6])
	}
	salida := "data/modelo.json"
	if len(os.Args) > 7 {
		salida = os.Args[7]
	}

	cb, err := comun.CargarCodebook("data/codebook.json")
	if err != nil {
		fmt.Println("Error al leer el codebook:", err)
		os.Exit(1)
	}

	inicio := time.Now()
	train, err := comun.CargarCSV(rutaTrain, cb, workers)
	if err != nil {
		fmt.Println("Error al cargar el entrenamiento:", err)
		os.Exit(1)
	}
	test, err := comun.CargarCSV(rutaTest, cb, workers)
	if err != nil {
		fmt.Println("Error al cargar la prueba:", err)
		os.Exit(1)
	}
	tCarga := time.Since(inicio)
	fmt.Printf("Carga: train %d filas (%d invalidas), test %d filas, %d columnas, %.3f s\n",
		len(train.X), train.Invalidas, len(test.X), len(train.Nombres), tCarga.Seconds())

	fmt.Printf("Entrenando: %d workers, %d epocas, tasa %.3f, lote %d\n", workers, epocas, tasa, lote)
	inicio = time.Now()
	w, perdidas := comun.Entrenar(train, workers, epocas, tasa, lote, func(e int, p float64) {
		if e == 1 || e%10 == 0 || e == epocas {
			fmt.Printf("  epoca %4d  perdida %.6f\n", e, p)
		}
	})
	tEntren := time.Since(inicio)
	fmt.Printf("Entrenamiento: %.3f s (%.1f ms/epoca)\n", tEntren.Seconds(), 1000*tEntren.Seconds()/float64(epocas))

	// evaluacion sobre la prueba real
	p := comun.Predecir(test.X, w, workers)
	m05 := comun.Evaluar(test.Y, p, 0.5)
	mF1 := comun.MejorUmbral(test.Y, p)
	fmt.Printf("Test (umbral 0.50): AUC %.4f  acc %.4f  prec %.4f  rec %.4f  F1 %.4f  [VP %d FP %d FN %d VN %d]\n",
		m05.AUC, m05.Accuracy, m05.Precision, m05.Recall, m05.F1, m05.VP, m05.FP, m05.FN, m05.VN)
	fmt.Printf("Test (umbral %.2f): AUC %.4f  acc %.4f  prec %.4f  rec %.4f  F1 %.4f  [VP %d FP %d FN %d VN %d]\n",
		mF1.Umbral, mF1.AUC, mF1.Accuracy, mF1.Precision, mF1.Recall, mF1.F1, mF1.VP, mF1.FP, mF1.FN, mF1.VN)

	type par struct {
		nombre string
		peso   float64
	}
	var pares []par
	for j, n := range train.Nombres {
		if n != "sesgo" {
			pares = append(pares, par{n, w[j]})
		}
	}
	sort.Slice(pares, func(i, j int) bool { return math.Abs(pares[i].peso) > math.Abs(pares[j].peso) })
	fmt.Println("Coeficientes con mayor efecto (odds ratio):")
	for _, q := range pares[:8] {
		fmt.Printf("  %-22s peso %+.4f  OR %.3f\n", q.nombre, q.peso, math.Exp(q.peso))
	}

	modelo := comun.Modelo{Columnas: train.Nombres, Pesos: w, Codebook: cb, Workers: workers, Epocas: epocas, Tasa: tasa,
		Lote: lote, FilasTrain: len(train.X), FilasTest: len(test.X), PerdidaFinal: perdidas[len(perdidas)-1],
		Metricas05: m05, MetricasF1: mF1}
	if err := comun.GuardarModelo(salida, modelo); err != nil {
		fmt.Println("No se pudo guardar el modelo:", err)
	} else {
		fmt.Println("Modelo guardado en", salida)
	}
	fmt.Printf("RESUMEN workers=%d carga_ms=%d entren_ms=%d epocas=%d perdida=%.6f auc=%.4f f1=%.4f\n",
		workers, tCarga.Milliseconds(), tEntren.Milliseconds(), epocas, perdidas[len(perdidas)-1], m05.AUC, mF1.F1)
}
