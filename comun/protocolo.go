package comun

import (
	"bufio"
	"encoding/json"
	"net"
	"time"
)

// mensajes coordinador <-> nodo: un JSON por linea
type Peticion struct {
	Op    string    `json:"op"`
	Pesos []float64 `json:"pesos,omitempty"`
	Ini   int       `json:"ini"`
	Fin   int       `json:"fin"`
}

type Respuesta struct {
	Nodo    string    `json:"nodo"`
	Filas   int       `json:"filas"`
	Grad    []float64 `json:"grad,omitempty"`
	Perdida float64   `json:"perdida"`
	Ms      float64   `json:"ms"`
	Error   string    `json:"error,omitempty"`
}

type Conexion struct {
	conn    net.Conn
	lector  *bufio.Reader
	timeout time.Duration
}

func NuevaConexion(conn net.Conn, timeout time.Duration) *Conexion {
	return &Conexion{conn: conn, lector: bufio.NewReaderSize(conn, 64*1024), timeout: timeout}
}

func Conectar(dir string, timeout time.Duration) (*Conexion, error) {
	conn, err := net.DialTimeout("tcp", dir, 3*time.Second)
	if err != nil {
		return nil, err
	}
	return NuevaConexion(conn, timeout), nil
}

func (c *Conexion) Enviar(v any) error {
	datos, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	_, err = c.conn.Write(append(datos, '\n'))
	return err
}

func (c *Conexion) Recibir(v any) error {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	linea, err := c.lector.ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(linea, v)
}

func (c *Conexion) Cerrar() error { return c.conn.Close() }

func (c *Conexion) Remota() string { return c.conn.RemoteAddr().String() }
