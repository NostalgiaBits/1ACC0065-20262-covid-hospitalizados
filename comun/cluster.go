package comun

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// coordinador del cluster: topologia en estrella, cada nodo solo habla con el coordinador
type Cluster struct {
	Dirs    []string
	Timeout time.Duration

	mu    sync.Mutex
	conns []*Conexion
}

type EstadoNodo struct {
	Dir         string  `json:"dir" bson:"dir"`
	Nodo        string  `json:"nodo" bson:"nodo"`
	Activo      bool    `json:"activo" bson:"activo"`
	Filas       int     `json:"filas" bson:"filas"`
	Peticiones  int     `json:"peticiones" bson:"peticiones"`
	MsCalculo   float64 `json:"ms_calculo" bson:"ms_calculo"`
	Fallos      int     `json:"fallos" bson:"fallos"`
	UltimoError string  `json:"ultimo_error,omitempty" bson:"ultimo_error,omitempty"`
}

type ResultadoCluster struct {
	Pesos          []float64    `json:"-"`
	Perdidas       []float64    `json:"perdidas"`
	Nodos          []EstadoNodo `json:"nodos"`
	Reasignaciones int          `json:"reasignaciones"`
	Filas          int          `json:"filas"`
	Ms             float64      `json:"ms"`
}

func NuevoCluster(dirs []string, timeout time.Duration) *Cluster {
	return &Cluster{Dirs: dirs, Timeout: timeout, conns: make([]*Conexion, len(dirs))}
}

func (c *Cluster) conexion(i int) (*Conexion, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conns[i] == nil {
		conn, err := Conectar(c.Dirs[i], c.Timeout)
		if err != nil {
			return nil, err
		}
		c.conns[i] = conn
	}
	return c.conns[i], nil
}

func (c *Cluster) descartar(i int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conns[i] != nil {
		c.conns[i].Cerrar()
		c.conns[i] = nil
	}
}

// una peticion a un nodo; si la conexion falla se descarta para reconectar luego
func (c *Cluster) pedir(i int, pet Peticion) (Respuesta, error) {
	conn, err := c.conexion(i)
	if err != nil {
		return Respuesta{}, err
	}
	var res Respuesta
	if err := conn.Enviar(pet); err != nil {
		c.descartar(i)
		return res, err
	}
	if err := conn.Recibir(&res); err != nil {
		c.descartar(i)
		return res, err
	}
	if res.Error != "" {
		return res, errors.New(res.Error)
	}
	return res, nil
}

// consulta el estado de todos los nodos en paralelo
func (c *Cluster) Estado() []EstadoNodo {
	estados := make([]EstadoNodo, len(c.Dirs))
	var wg sync.WaitGroup
	for i := range c.Dirs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			inicio := time.Now()
			res, err := c.pedir(i, Peticion{Op: "salud"})
			estados[i] = EstadoNodo{Dir: c.Dirs[i], Nodo: res.Nodo, Activo: err == nil, Filas: res.Filas,
				MsCalculo: float64(time.Since(inicio).Microseconds()) / 1000}
			if err != nil {
				estados[i].UltimoError = err.Error()
			}
		}(i)
	}
	wg.Wait()
	return estados
}

type parte struct {
	nodo     int
	ini, fin int
	res      Respuesta
	err      error
}

// descenso de gradiente distribuido: por epoca el coordinador envia w, cada nodo devuelve el gradiente de su rango
// y el coordinador suma en orden de rango y actualiza w (unica escritura de los pesos)
func (c *Cluster) Entrenar(nodos, dim, epocas int, tasa float64, informe func(epoca int, perdida float64)) (*ResultadoCluster, error) {
	if nodos < 1 || nodos > len(c.Dirs) {
		nodos = len(c.Dirs)
	}
	inicio := time.Now()
	r := &ResultadoCluster{Nodos: make([]EstadoNodo, nodos)}
	activos := make([]bool, nodos)
	n := -1
	for i := 0; i < nodos; i++ {
		r.Nodos[i].Dir = c.Dirs[i]
		res, err := c.pedir(i, Peticion{Op: "info"})
		if err != nil {
			r.Nodos[i].UltimoError = err.Error()
			continue
		}
		activos[i] = true
		r.Nodos[i].Nodo, r.Nodos[i].Filas, r.Nodos[i].Activo = res.Nodo, res.Filas, true
		if n == -1 {
			n = res.Filas
		} else if res.Filas != n {
			return nil, fmt.Errorf("los nodos tienen distinto numero de filas (%d y %d)", n, res.Filas)
		}
	}
	if n <= 0 {
		return nil, errors.New("ningun nodo disponible")
	}
	r.Filas = n

	w := make([]float64, dim)
	tam := (n + nodos - 1) / nodos
	for e := 0; e < epocas; e++ {
		// fan-out: un rango disjunto por nodo
		resultados := make(chan parte, nodos)
		var wg sync.WaitGroup
		for k := 0; k < nodos; k++ {
			ini, fin := k*tam, (k+1)*tam
			if fin > n {
				fin = n
			}
			wg.Add(1)
			go func(k, ini, fin int) {
				defer wg.Done()
				p := parte{nodo: k, ini: ini, fin: fin, err: errors.New("nodo inactivo")}
				if activos[k] {
					p.res, p.err = c.pedir(k, Peticion{Op: "gradiente", Pesos: w, Ini: ini, Fin: fin})
				}
				resultados <- p
			}(k, ini, fin)
		}
		go func() { wg.Wait(); close(resultados) }()

		// fan-in
		partes := make([]parte, nodos)
		for p := range resultados {
			partes[p.nodo] = p
		}
		// rangos de nodos caidos: se reasignan al siguiente nodo vivo
		for k := range partes {
			if partes[k].err == nil {
				r.Nodos[k].Peticiones++
				r.Nodos[k].MsCalculo += partes[k].res.Ms
				continue
			}
			if activos[k] {
				activos[k] = false
				r.Nodos[k].Activo = false
				r.Nodos[k].Fallos++
				r.Nodos[k].UltimoError = partes[k].err.Error()
			}
			ok := false
			for j := 1; j < nodos && !ok; j++ {
				otro := (k + j) % nodos
				if !activos[otro] {
					continue
				}
				res, err := c.pedir(otro, Peticion{Op: "gradiente", Pesos: w, Ini: partes[k].ini, Fin: partes[k].fin})
				if err != nil {
					activos[otro] = false
					r.Nodos[otro].Activo = false
					r.Nodos[otro].Fallos++
					r.Nodos[otro].UltimoError = err.Error()
					continue
				}
				partes[k].res, partes[k].err, ok = res, nil, true
				r.Reasignaciones++
				r.Nodos[otro].Peticiones++
				r.Nodos[otro].MsCalculo += res.Ms
			}
			if !ok {
				return nil, fmt.Errorf("epoca %d: no queda ningun nodo para el rango [%d, %d)", e+1, partes[k].ini, partes[k].fin)
			}
		}

		suma := make([]float64, dim)
		perdida := 0.0
		for k := 0; k < nodos; k++ { // orden fijo
			for j := range suma {
				suma[j] += partes[k].res.Grad[j]
			}
			perdida += partes[k].res.Perdida
		}
		for j := range w {
			w[j] -= tasa * suma[j] / float64(n)
		}
		perdida /= float64(n)
		r.Perdidas = append(r.Perdidas, perdida)
		if informe != nil {
			informe(e+1, perdida)
		}
	}
	r.Pesos = w
	r.Ms = float64(time.Since(inicio).Microseconds()) / 1000
	return r, nil
}
