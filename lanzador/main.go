// Prueba un nodo directamente desde el host, sin pasar por la API.
// Uso: go run ./lanzador (pide puerto y operacion) o go run ./lanzador 19091 gradiente
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"covid-hospitalizados/comun"
)

func leer(lector *bufio.Reader, mensaje string) string {
	fmt.Print(mensaje)
	texto, _ := lector.ReadString('\n')
	return strings.TrimSpace(texto)
}

func main() {
	lector := bufio.NewReader(os.Stdin)
	var puerto, op string
	if len(os.Args) > 2 {
		puerto, op = os.Args[1], os.Args[2]
	} else {
		puerto = leer(lector, "Ingrese el puerto remoto: ")
		op = leer(lector, "Ingrese la operacion (info | gradiente): ")
	}

	conn, err := comun.Conectar("localhost:"+puerto, 2*time.Minute)
	if err != nil {
		fmt.Println("No se pudo conectar:", err)
		return
	}
	defer conn.Cerrar()

	var info comun.Respuesta
	if err := conn.Enviar(comun.Peticion{Op: "info"}); err != nil {
		fmt.Println("Error al enviar:", err)
		return
	}
	if err := conn.Recibir(&info); err != nil {
		fmt.Println("Error al recibir:", err)
		return
	}
	fmt.Printf("Nodo %s: %d filas\n", info.Nodo, info.Filas)
	if op != "gradiente" {
		return
	}

	cb, err := comun.CargarCodebook("data/codebook.json")
	if err != nil {
		fmt.Println("codebook:", err)
		return
	}
	pet := comun.Peticion{Op: "gradiente", Pesos: make([]float64, cb.Dimension()), Ini: 0, Fin: info.Filas}
	inicio := time.Now()
	var res comun.Respuesta
	if err := conn.Enviar(pet); err != nil {
		fmt.Println("Error al enviar:", err)
		return
	}
	if err := conn.Recibir(&res); err != nil {
		fmt.Println("Error al recibir:", err)
		return
	}
	if res.Error != "" {
		fmt.Println("El nodo respondio con error:", res.Error)
		return
	}
	fmt.Printf("Gradiente de %d filas con pesos en cero: perdida media %.6f\n", info.Filas, res.Perdida/float64(info.Filas))
	fmt.Printf("Calculo en el nodo: %.1f ms · ida y vuelta: %.1f ms\n", res.Ms, float64(time.Since(inicio).Microseconds())/1000)
}
