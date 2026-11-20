// Nodo ML del cluster: carga el CSV y calcula gradientes parciales para el coordinador por TCP.
// Variables de entorno: NODO_ID, PUERTO (9091), DATOS (data/train_1M.csv), CODEBOOK, WORKERS (4)
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"covid-hospitalizados/comun"
)

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func main() {
	nombre := entorno("NODO_ID", "nodo1")
	puerto := entorno("PUERTO", "9091")
	workers, err := strconv.Atoi(entorno("WORKERS", "4"))
	if err != nil || workers < 1 {
		log.Fatal("WORKERS debe ser un entero >= 1")
	}

	cb, err := comun.CargarCodebook(entorno("CODEBOOK", "data/codebook.json"))
	if err != nil {
		log.Fatal("codebook: ", err)
	}
	inicio := time.Now()
	datos, err := comun.CargarCSV(entorno("DATOS", "data/train_1M.csv"), cb, workers)
	if err != nil {
		log.Fatal("datos: ", err)
	}
	log.Printf("%s: %d filas cargadas en %.2f s con %d workers", nombre, len(datos.X), time.Since(inicio).Seconds(), workers)

	ls, err := net.Listen("tcp", ":"+puerto)
	if err != nil {
		log.Fatal(err)
	}
	defer ls.Close()
	fmt.Println(direccionLocal(puerto))
	log.Printf("%s: escuchando en el puerto %s", nombre, puerto)

	for {
		conn, err := ls.Accept()
		if err != nil {
			log.Println("error al aceptar:", err)
			continue
		}
		go comun.AtenderNodo(conn, datos, nombre, workers)
	}
}

// ip del contenedor en la red del compose
func direccionLocal(puerto string) string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil {
			return ip.IP.String() + ":" + puerto
		}
	}
	return "127.0.0.1:" + puerto
}
