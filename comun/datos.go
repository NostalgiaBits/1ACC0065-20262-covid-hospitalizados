package comun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// categorias en orden fijo (data/codebook.json); la primera de cada lista es la de referencia
type Codebook struct {
	Sexo         []string `json:"sexo"`
	Dosis        []string `json:"dosis_previas"`
	Fabricante   []string `json:"fabricante"`
	Departamento []string `json:"departamento"`
	Periodo      []string `json:"periodo"`
	EdadMin      int      `json:"edad_min"`
	EdadMax      int      `json:"edad_max"`
	EdadMedia    float64  `json:"edad_media"`
	EdadDesv     float64  `json:"edad_desv"`

	pos map[string]int
}

type Datos struct {
	X         [][]float64
	Y         []float64
	Nombres   []string
	Lineas    int
	Invalidas int
}

const TamBloque = 4 * 1024 * 1024

func CargarCodebook(ruta string) (*Codebook, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var cb Codebook
	if err := json.Unmarshal(contenido, &cb); err != nil {
		return nil, err
	}
	if cb.EdadDesv <= 0 {
		return nil, errors.New("codebook sin edad_media/edad_desv: ejecutar python/aumento.py")
	}
	cb.pos = map[string]int{}
	for i, n := range cb.Nombres() {
		cb.pos[n] = i
	}
	return &cb, nil
}

func (cb *Codebook) Nombres() []string {
	n := []string{"sesgo", "edad_z"}
	for _, c := range cb.Sexo[1:] {
		n = append(n, "sexo="+c)
	}
	for _, c := range cb.Dosis[1:] {
		n = append(n, "dosis="+c)
	}
	for _, c := range cb.Fabricante[1:] {
		n = append(n, "fabricante="+c)
	}
	for _, c := range cb.Departamento[1:] {
		n = append(n, "dep="+c)
	}
	for _, c := range cb.Periodo[1:] {
		n = append(n, "periodo="+c)
	}
	return n
}

func (cb *Codebook) Dimension() int { return len(cb.Nombres()) }

// linea "id_persona,edad,sexo,dosis_previas,fabricante,departamento,periodo,fallecido" -> fila x, etiqueta y
func (cb *Codebook) Codificar(linea string, x []float64) (float64, error) {
	campos := strings.Split(linea, ",")
	if len(campos) != 8 {
		return 0, fmt.Errorf("se esperaban 8 campos, hay %d", len(campos))
	}
	edad, err := strconv.Atoi(campos[1])
	if err != nil || edad < cb.EdadMin || edad > cb.EdadMax {
		return 0, fmt.Errorf("edad invalida %q", campos[1])
	}
	dosis, err := strconv.Atoi(campos[3])
	if err != nil || dosis < 0 || dosis >= len(cb.Dosis) {
		return 0, fmt.Errorf("dosis invalida %q", campos[3])
	}
	y, err := strconv.Atoi(campos[7])
	if err != nil || (y != 0 && y != 1) {
		return 0, fmt.Errorf("fallecido invalido %q", campos[7])
	}
	for i := range x {
		x[i] = 0
	}
	x[0] = 1
	x[1] = (float64(edad) - cb.EdadMedia) / cb.EdadDesv
	for _, par := range [][2]string{
		{"sexo", campos[2]}, {"dosis", cb.Dosis[dosis]}, {"fabricante", campos[4]}, {"dep", campos[5]}, {"periodo", campos[6]},
	} {
		if p, ok := cb.pos[par[0]+"="+par[1]]; ok {
			x[p] = 1
		} else if !cb.esReferencia(par[0], par[1]) {
			return 0, fmt.Errorf("%s desconocido %q", par[0], par[1])
		}
	}
	return float64(y), nil
}

func (cb *Codebook) esReferencia(grupo, valor string) bool {
	switch grupo {
	case "sexo":
		return valor == cb.Sexo[0]
	case "dosis":
		return valor == cb.Dosis[0]
	case "fabricante":
		return valor == cb.Fabricante[0]
	case "dep":
		return valor == cb.Departamento[0]
	case "periodo":
		return valor == cb.Periodo[0]
	}
	return false
}

type bloque struct {
	indice int
	datos  []byte
}

type resultado struct {
	indice    int
	x         [][]float64
	y         []float64
	lineas    int
	invalidas int
}

// lee el archivo por bloques y corta siempre en un salto de linea
func productor(archivo *os.File, tam int, salida chan<- bloque) error {
	defer close(salida)
	buf := make([]byte, tam)
	resto := []byte{}
	indice := 0
	cabecera := true
	for {
		n, err := archivo.Read(buf)
		if n > 0 {
			datos := append(resto, buf[:n]...)
			corte := bytes.LastIndexByte(datos, '\n')
			if corte < 0 {
				resto = datos
				continue
			}
			listo := datos[:corte+1]
			resto = append([]byte{}, datos[corte+1:]...)
			if cabecera {
				fin := bytes.IndexByte(listo, '\n')
				listo = listo[fin+1:]
				cabecera = false
			}
			if len(listo) > 0 {
				salida <- bloque{indice, listo}
				indice++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	if len(bytes.TrimSpace(resto)) > 0 {
		salida <- bloque{indice, append(resto, '\n')}
	}
	return nil
}

// cada worker parsea y codifica sus bloques en memoria propia
func worker(cb *Codebook, entrada <-chan bloque, salida chan<- resultado, wg *sync.WaitGroup) {
	defer wg.Done()
	d := cb.Dimension()
	for b := range entrada {
		lineas := bytes.Split(bytes.TrimRight(b.datos, "\n"), []byte{'\n'})
		plano := make([]float64, len(lineas)*d)
		r := resultado{indice: b.indice, x: make([][]float64, 0, len(lineas)), y: make([]float64, 0, len(lineas))}
		for _, l := range lineas {
			linea := strings.TrimRight(string(l), "\r")
			if linea == "" {
				continue
			}
			r.lineas++
			fila := plano[len(r.x)*d : (len(r.x)+1)*d]
			y, err := cb.Codificar(linea, fila)
			if err != nil {
				r.invalidas++
				continue
			}
			r.x = append(r.x, fila)
			r.y = append(r.y, y)
		}
		salida <- r
	}
}

func CargarCSV(ruta string, cb *Codebook, workers int) (*Datos, error) {
	return CargarCSVBloques(ruta, cb, workers, TamBloque)
}

// productor -> workers -> agregador
func CargarCSVBloques(ruta string, cb *Codebook, workers, tamBloque int) (*Datos, error) {
	if workers < 1 {
		return nil, errors.New("workers debe ser >= 1")
	}
	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer archivo.Close()

	bloques := make(chan bloque, workers)
	resultados := make(chan resultado, workers)
	var wg sync.WaitGroup
	errProductor := make(chan error, 1)

	go func() { errProductor <- productor(archivo, tamBloque, bloques) }()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker(cb, bloques, resultados, &wg)
	}
	go func() {
		wg.Wait()
		close(resultados)
	}()

	var partes []resultado
	for r := range resultados {
		partes = append(partes, r)
	}
	if err := <-errProductor; err != nil {
		return nil, err
	}
	// orden del archivo
	sort.Slice(partes, func(i, j int) bool { return partes[i].indice < partes[j].indice })

	datos := &Datos{Nombres: cb.Nombres()}
	for _, p := range partes {
		datos.X = append(datos.X, p.x...)
		datos.Y = append(datos.Y, p.y...)
		datos.Lineas += p.lineas
		datos.Invalidas += p.invalidas
	}
	return datos, nil
}
