package comun

import (
	"math"
	"net"
	"testing"
	"time"
)

func nodoPrueba(t *testing.T, d *Datos, nombre string) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go AtenderNodo(conn, d, nombre, 3)
		}
	}()
	return ln.Addr().String()
}

// nodo que responde info y se cae al primer gradiente
func nodoQueFalla(t *testing.T, filas int) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		c := NuevaConexion(conn, time.Second)
		var pet Peticion
		c.Recibir(&pet)
		c.Enviar(Respuesta{Nodo: "falla", Filas: filas})
		c.Recibir(&pet)
		conn.Close()
		ln.Close()
	}()
	return ln.Addr().String()
}

func iguales(t *testing.T, a, b []float64, tol float64) {
	for j := range a {
		if math.Abs(a[j]-b[j]) > tol {
			t.Fatalf("difieren en %d: %v vs %v", j, a[j], b[j])
		}
	}
}

// distribuido en 3 nodos = entrenamiento local
func TestClusterIgualALocal(t *testing.T) {
	d := datosPrueba(1000)
	dirs := []string{nodoPrueba(t, d, "n1"), nodoPrueba(t, d, "n2"), nodoPrueba(t, d, "n3")}
	c := NuevoCluster(dirs, 5*time.Second)
	r, err := c.Entrenar(3, len(d.Nombres), 40, 0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, perdidas := Entrenar(d, 1, 40, 0.5, 0, nil)
	iguales(t, r.Pesos, w, 1e-9)
	iguales(t, r.Perdidas, perdidas, 1e-9)
	for _, nodo := range r.Nodos {
		if nodo.Peticiones != 40 {
			t.Errorf("%s atendio %d peticiones", nodo.Dir, nodo.Peticiones)
		}
	}
}

// un nodo se cae a mitad: su rango se reasigna y el resultado no cambia
func TestClusterReasignaNodoCaido(t *testing.T) {
	d := datosPrueba(900)
	dirs := []string{nodoPrueba(t, d, "n1"), nodoQueFalla(t, len(d.X)), nodoPrueba(t, d, "n3")}
	c := NuevoCluster(dirs, 2*time.Second)
	r, err := c.Entrenar(3, len(d.Nombres), 10, 0.5, nil)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := Entrenar(d, 1, 10, 0.5, 0, nil)
	iguales(t, r.Pesos, w, 1e-9)
	if r.Reasignaciones != 10 || r.Nodos[1].Activo || r.Nodos[1].Fallos != 1 {
		t.Errorf("reasignaciones %d, nodo caido %+v", r.Reasignaciones, r.Nodos[1])
	}
}

func TestClusterSinNodos(t *testing.T) {
	c := NuevoCluster([]string{"127.0.0.1:1"}, time.Second)
	if _, err := c.Entrenar(1, 2, 1, 0.5, nil); err == nil {
		t.Error("deberia fallar sin nodos")
	}
}

func TestCodificarPaciente(t *testing.T) {
	cb := codebookPrueba()
	x := make([]float64, cb.Dimension())
	err := cb.CodificarPaciente(Paciente{Edad: 70, Sexo: "m", DosisPrevias: 5, Departamento: "cusco", Periodo: "2021-S1"}, x)
	if err != nil {
		t.Fatal(err)
	}
	if x[cb.pos["sexo=M"]] != 1 || x[cb.pos["dosis=3+"]] != 1 || x[cb.pos["dep=CUSCO"]] != 1 {
		t.Errorf("codificacion incorrecta: %v", x)
	}
	if cb.CodificarPaciente(Paciente{Edad: 200, Sexo: "M", Departamento: "LIMA", Periodo: "2021-S1"}, x) == nil {
		t.Error("edad fuera de rango aceptada")
	}
}
