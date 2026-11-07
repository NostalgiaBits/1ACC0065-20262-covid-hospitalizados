// Uso: go run ./cargador <archivo.csv> <workers> [codebook.json]
package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"covid-hospitalizados/comun"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Uso: cargador <archivo.csv> <workers> [codebook.json]")
		os.Exit(1)
	}
	ruta := os.Args[1]
	workers, err := strconv.Atoi(os.Args[2])
	if err != nil || workers < 1 {
		fmt.Println("workers debe ser un entero >= 1")
		os.Exit(1)
	}
	codebook := "data/codebook.json"
	if len(os.Args) > 3 {
		codebook = os.Args[3]
	}

	cb, err := comun.CargarCodebook(codebook)
	if err != nil {
		fmt.Println("Error al leer el codebook:", err)
		os.Exit(1)
	}

	fmt.Printf("Cargando %s con %d workers (bloques de %d KB)...\n", ruta, workers, comun.TamBloque/1024)
	inicio := time.Now()
	datos, err := comun.CargarCSV(ruta, cb, workers)
	if err != nil {
		fmt.Println("Error al cargar:", err)
		os.Exit(1)
	}
	duracion := time.Since(inicio)

	fallecidos := 0.0
	for _, y := range datos.Y {
		fallecidos += y
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	fmt.Printf("Lineas leidas      : %d\n", datos.Lineas)
	fmt.Printf("Filas validas      : %d\n", len(datos.X))
	fmt.Printf("Filas invalidas    : %d\n", datos.Invalidas)
	fmt.Printf("Columnas por fila  : %d (%v ...)\n", len(datos.Nombres), datos.Nombres[:6])
	fmt.Printf("Tasa de fallecidos : %.2f %%\n", 100*fallecidos/float64(len(datos.Y)))
	fmt.Printf("Tiempo de carga    : %.3f s  (%.0f filas/s)\n", duracion.Seconds(), float64(datos.Lineas)/duracion.Seconds())
	fmt.Printf("Memoria en uso     : %.0f MB\n", float64(mem.Alloc)/1024/1024)
	fmt.Printf("RESUMEN workers=%d carga_ms=%d filas=%d invalidas=%d mem_mb=%.0f\n",
		workers, duracion.Milliseconds(), len(datos.X), datos.Invalidas, float64(mem.Alloc)/1024/1024)
}
