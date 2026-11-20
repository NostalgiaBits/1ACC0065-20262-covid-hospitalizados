package comun

import (
	"io"
	"log"
	"net"
	"time"
)

// atiende las peticiones de un coordinador sobre una conexion persistente
func AtenderNodo(conn net.Conn, d *Datos, nombre string, workers int) {
	defer conn.Close()
	c := NuevaConexion(conn, 10*time.Minute)
	for {
		var pet Peticion
		if err := c.Recibir(&pet); err != nil {
			if err != io.EOF {
				log.Printf("%s: conexion %s cerrada: %v", nombre, c.Remota(), err)
			}
			return
		}
		inicio := time.Now()
		res := Respuesta{Nodo: nombre, Filas: len(d.X)}
		switch pet.Op {
		case "info", "salud":
		case "gradiente":
			if pet.Ini < 0 || pet.Fin > len(d.X) || pet.Ini > pet.Fin || len(pet.Pesos) != len(d.Nombres) {
				res.Error = "rango o pesos invalidos"
				break
			}
			res.Grad, res.Perdida = GradienteParalelo(d, pet.Pesos, pet.Ini, pet.Fin, workers)
		default:
			res.Error = "operacion desconocida: " + pet.Op
		}
		res.Ms = float64(time.Since(inicio).Microseconds()) / 1000
		if pet.Op == "gradiente" {
			log.Printf("%s: gradiente filas [%d, %d) en %.1f ms", nombre, pet.Ini, pet.Fin, res.Ms)
		} else {
			log.Printf("%s: %s desde %s", nombre, pet.Op, c.Remota())
		}
		if err := c.Enviar(res); err != nil {
			log.Printf("%s: no se pudo responder: %v", nombre, err)
			return
		}
	}
}
