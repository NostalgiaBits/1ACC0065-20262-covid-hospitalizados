// API coordinadora: predicciones, entrenamiento distribuido en el cluster TCP, MongoDB y Redis.
// Variables de entorno: ver .env.example
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"covid-hospitalizados/comun"
)

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

type Servidor struct {
	cb      *comun.Codebook
	cluster *comun.Cluster
	store   *Store
	test    *comun.Datos
	secreto []byte

	mu      sync.RWMutex
	modelo  *comun.Modelo
	version int
	origen  string

	entrenando sync.Mutex
	metricas   *Metricas
}

func main() {
	cb, err := comun.CargarCodebook(entorno("CODEBOOK", "data/codebook.json"))
	if err != nil {
		log.Fatal("codebook: ", err)
	}
	s := &Servidor{cb: cb, metricas: NuevasMetricas()}

	s.secreto = []byte(os.Getenv("JWT_SECRETO"))
	if len(s.secreto) == 0 {
		b := make([]byte, 32)
		rand.Read(b)
		s.secreto = []byte(hex.EncodeToString(b))
		log.Println("AVISO: JWT_SECRETO no definido; se uso uno aleatorio (los tokens caducan al reiniciar)")
	}

	var dirs []string
	for _, d := range strings.Split(entorno("NODOS", "localhost:19091,localhost:29091,localhost:39091"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	s.cluster = comun.NuevoCluster(dirs, 30*time.Second)

	s.store = ConectarStore(os.Getenv("MONGO_URI"), entorno("MONGO_DB", "covid"), os.Getenv("REDIS_ADDR"))
	s.cargarModelo(entorno("MODELO", "data/modelo.json"))
	s.sembrarAdmin(os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD"))

	if ruta := entorno("TEST", "data/test.csv"); ruta != "" {
		if s.test, err = comun.CargarCSV(ruta, cb, 4); err != nil {
			log.Println("AVISO: sin conjunto de prueba, /entrenar no calculara metricas:", err)
		} else {
			log.Printf("prueba: %d filas reales", len(s.test.X))
		}
	}

	puerto := entorno("PUERTO", "8080")
	log.Printf("API en :%s · nodos %v · mongo %v · redis %v · modelo v%d (%s)",
		puerto, dirs, s.store.Mongo != nil, s.store.Redis != nil, s.version, s.origen)
	srv := &http.Server{Addr: ":" + puerto, Handler: s, ReadTimeout: 10 * time.Second, WriteTimeout: 5 * time.Minute}
	log.Fatal(srv.ListenAndServe())
}

// modelo vigente: Redis -> MongoDB -> archivo
func (s *Servidor) cargarModelo(archivo string) {
	if m, v, ok := s.store.ModeloRedis(); ok {
		s.activar(m, v, "redis")
		return
	}
	if m, v, ok := s.store.UltimoModelo(); ok {
		s.activar(m, v, "mongodb")
		s.store.GuardarModeloRedis(m, v)
		return
	}
	contenido, err := os.ReadFile(archivo)
	if err != nil {
		log.Fatal("modelo: ", err)
	}
	var m comun.Modelo
	if err := json.Unmarshal(contenido, &m); err != nil {
		log.Fatal("modelo: ", err)
	}
	if len(m.Pesos) != s.cb.Dimension() {
		log.Fatalf("modelo con %d pesos, el codebook espera %d", len(m.Pesos), s.cb.Dimension())
	}
	s.activar(&m, 1, "archivo")
	s.store.GuardarModeloRedis(&m, 1)
}

func (s *Servidor) activar(m *comun.Modelo, version int, origen string) {
	s.mu.Lock()
	s.modelo, s.version, s.origen = m, version, origen
	s.mu.Unlock()
}

func (s *Servidor) vigente() (*comun.Modelo, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.modelo, s.version
}
