package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"covid-hospitalizados/comun"
)

//go:embed openapi.yaml
var especificacion []byte

const aviso = "Apoya la priorizacion de recursos; no reemplaza el juicio clinico ni debe usarse para negar atencion."

type ruta map[string]http.HandlerFunc

func (s *Servidor) rutas() map[string]ruta {
	return map[string]ruta{
		"/salud":          {"GET": s.salud},
		"/registro":       {"POST": s.registro},
		"/login":          {"POST": s.login},
		"/predecir":       {"POST": s.predecir},
		"/modelo":         {"GET": s.verModelo},
		"/cluster":        {"GET": s.verCluster},
		"/metricas":       {"GET": s.verMetricas},
		"/entrenar":       {"POST": s.protegido("admin", s.entrenar)},
		"/entrenamientos": {"GET": s.protegido("", s.verEntrenamientos)},
		"/predicciones":   {"GET": s.protegido("", s.verPredicciones)},
		"/openapi.yaml":   {"GET": s.openapi},
		"/docs":           {"GET": s.docs},
	}
}

type registro struct {
	http.ResponseWriter
	estado int
}

func (r *registro) WriteHeader(c int) { r.estado = c; r.ResponseWriter.WriteHeader(c) }

// un solo handler con enrutado a mano
func (s *Servidor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	inicio := time.Now()
	rw := &registro{ResponseWriter: w, estado: http.StatusOK}
	defer func() {
		if r.URL.Path != "/salud" {
			log.Printf("%s %s %d %.2f ms", r.Method, r.URL.Path, rw.estado, float64(time.Since(inicio).Microseconds())/1000)
		}
	}()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if r.Method == http.MethodOptions {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	camino := strings.TrimSuffix(r.URL.Path, "/")
	if camino == "" {
		camino = "/docs"
	}
	metodos, ok := s.rutas()[camino]
	if !ok {
		fallo(rw, "ruta no encontrada: "+r.URL.Path, http.StatusNotFound)
		return
	}
	h, ok := metodos[r.Method]
	if !ok {
		permitidos := make([]string, 0, len(metodos))
		for m := range metodos {
			permitidos = append(permitidos, m)
		}
		rw.Header().Set("Allow", strings.Join(permitidos, ", "))
		fallo(rw, "metodo no permitido", http.StatusMethodNotAllowed)
		return
	}
	h(rw, r)
}

func responder(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(v)
}

func fallo(w http.ResponseWriter, mensaje string, codigo int) {
	responder(w, codigo, map[string]string{"error": mensaje})
}

func leerJSON(w http.ResponseWriter, r *http.Request, destino any) bool {
	tipo, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if tipo != "application/json" {
		fallo(w, "usa Content-Type: application/json", http.StatusUnsupportedMediaType)
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		fallo(w, "JSON invalido: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// ---------- salud, usuarios

func (s *Servidor) salud(w http.ResponseWriter, r *http.Request) {
	mongoOK, redisOK := s.store.Estado()
	activos := 0
	for _, n := range s.cluster.Estado() {
		if n.Activo {
			activos++
		}
	}
	_, v := s.vigente()
	responder(w, http.StatusOK, map[string]any{"estado": "ok", "mongo": mongoOK, "redis": redisOK,
		"nodos_activos": activos, "nodos_total": len(s.cluster.Dirs), "modelo_version": v, "hora": time.Now().Format(time.RFC3339)})
}

func (s *Servidor) registro(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Nombre   string `json:"nombre"`
		Password string `json:"password"`
	}
	if !leerJSON(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !strings.Contains(in.Email, "@") || !strings.Contains(in.Email, ".") {
		fallo(w, "email invalido", http.StatusBadRequest)
		return
	}
	if len(in.Password) < 8 {
		fallo(w, "la contraseña debe tener al menos 8 caracteres", http.StatusBadRequest)
		return
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		fallo(w, "no se pudo registrar", http.StatusInternalServerError)
		return
	}
	u := Usuario{Email: in.Email, Nombre: strings.TrimSpace(in.Nombre), Hash: hash, Rol: "usuario", Creado: time.Now()}
	if err := s.store.CrearUsuario(u); err == ErrExiste {
		fallo(w, "el email ya esta registrado", http.StatusConflict)
		return
	} else if err != nil {
		fallo(w, "no se pudo registrar", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", "/login")
	responder(w, http.StatusCreated, u)
}

func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !leerJSON(w, r, &in) {
		return
	}
	u, err := s.autenticar(in.Email, in.Password)
	if err != nil {
		fallo(w, err.Error(), http.StatusUnauthorized)
		return
	}
	token, expira, err := s.emitirToken(u)
	if err != nil {
		fallo(w, "no se pudo emitir el token", http.StatusInternalServerError)
		return
	}
	responder(w, http.StatusOK, map[string]any{"token": token, "tipo": "Bearer", "expira": expira.Format(time.RFC3339), "rol": u.Rol, "email": u.Email})
}

// ---------- prediccion

type factor struct {
	Variable     string  `json:"variable"`
	Contribucion float64 `json:"contribucion"`
	OddsRatio    float64 `json:"odds_ratio"`
}

type respuestaPrediccion struct {
	Probabilidad  float64  `json:"probabilidad"`
	Porcentaje    string   `json:"porcentaje"`
	Riesgo        string   `json:"riesgo"`
	Prioritario   bool     `json:"prioritario"`
	Umbral        float64  `json:"umbral"`
	Factores      []factor `json:"factores"`
	ModeloVersion int      `json:"modelo_version"`
	Cache         string   `json:"cache"`
	LatenciaMs    float64  `json:"latencia_ms"`
	Aviso         string   `json:"aviso"`
}

func (s *Servidor) predecir(w http.ResponseWriter, r *http.Request) {
	inicio := time.Now()
	var p comun.Paciente
	if !leerJSON(w, r, &p) {
		s.metricas.Error()
		return
	}
	p.Sexo, p.Fabricante = strings.ToUpper(strings.TrimSpace(p.Sexo)), strings.ToUpper(strings.TrimSpace(p.Fabricante))
	p.Departamento, p.Periodo = strings.ToUpper(strings.TrimSpace(p.Departamento)), strings.ToUpper(strings.TrimSpace(p.Periodo))
	if p.Sexo == "" || p.Departamento == "" || p.Periodo == "" {
		s.metricas.Error()
		fallo(w, "faltan campos: edad, sexo, dosis_previas, departamento y periodo son obligatorios", http.StatusBadRequest)
		return
	}
	if p.DosisPrevias < 0 {
		s.metricas.Error()
		fallo(w, "dosis_previas no puede ser negativa", http.StatusBadRequest)
		return
	}
	if p.DosisPrevias > 3 {
		p.DosisPrevias = 3
	}
	if p.Fabricante == "" {
		p.Fabricante = "NINGUNA"
	}

	m, version := s.vigente()
	clave := fmt.Sprintf("pred:v%d:%d|%s|%d|%s|%s|%s", version, p.Edad, p.Sexo, p.DosisPrevias, p.Fabricante, p.Departamento, p.Periodo)
	var res respuestaPrediccion
	hit := false
	if datos, ok := s.store.CacheGet(clave); ok && json.Unmarshal(datos, &res) == nil {
		hit = true
	} else {
		x := make([]float64, s.cb.Dimension())
		if err := s.cb.CodificarPaciente(p, x); err != nil {
			s.metricas.Error()
			fallo(w, err.Error()+" (valores validos en GET /modelo)", http.StatusBadRequest)
			return
		}
		res = calcular(m, x, version)
		datos, _ := json.Marshal(res)
		s.store.CacheSet(clave, datos)
	}
	res.Cache = map[bool]string{true: "hit", false: "miss"}[hit]
	res.LatenciaMs = float64(time.Since(inicio).Microseconds()) / 1000
	w.Header().Set("X-Latencia-ms", strconv.FormatFloat(res.LatenciaMs, 'f', 3, 64))
	responder(w, http.StatusOK, res)

	s.metricas.Registrar(res.LatenciaMs, hit)
	go func(p comun.Paciente, res respuestaPrediccion) { // fuera del camino de la respuesta
		s.store.RegistrarLatencia(res.LatenciaMs, hit)
		s.store.GuardarPrediccion(Prediccion{Fecha: time.Now(), Entrada: p, Probabilidad: res.Probabilidad, Riesgo: res.Riesgo,
			ModeloVersion: res.ModeloVersion, LatenciaMs: res.LatenciaMs, Cache: hit})
	}(p, res)
}

func calcular(m *comun.Modelo, x []float64, version int) respuestaPrediccion {
	z := 0.0
	var factores []factor
	for j, w := range m.Pesos {
		z += w * x[j]
		if j > 0 && x[j] != 0 {
			factores = append(factores, factor{Variable: m.Columnas[j], Contribucion: redondear(w * x[j]), OddsRatio: redondear(math.Exp(w))})
		}
	}
	sort.Slice(factores, func(a, b int) bool { return math.Abs(factores[a].Contribucion) > math.Abs(factores[b].Contribucion) })
	if len(factores) > 4 {
		factores = factores[:4]
	}
	prob := comun.Sigmoide(z)
	umbral := m.MetricasF1.Umbral
	if umbral == 0 {
		umbral = 0.5
	}
	riesgo := "bajo"
	if prob >= 0.5 {
		riesgo = "alto"
	} else if prob >= umbral {
		riesgo = "medio"
	}
	return respuestaPrediccion{Probabilidad: redondear(prob), Porcentaje: fmt.Sprintf("%.1f %%", 100*prob), Riesgo: riesgo,
		Prioritario: prob >= umbral, Umbral: redondear(umbral), Factores: factores, ModeloVersion: version, Aviso: aviso}
}

// ---------- entrenamiento distribuido

func (s *Servidor) entrenar(w http.ResponseWriter, r *http.Request, c *Claims) {
	in := struct {
		Epocas int     `json:"epocas"`
		Tasa   float64 `json:"tasa"`
		Nodos  int     `json:"nodos"`
	}{Epocas: 100, Tasa: 1.0, Nodos: len(s.cluster.Dirs)}
	if r.ContentLength != 0 && !leerJSON(w, r, &in) {
		return
	}
	if in.Epocas < 1 || in.Epocas > 2000 || in.Tasa <= 0 || in.Tasa > 10 || in.Nodos < 1 || in.Nodos > len(s.cluster.Dirs) {
		fallo(w, fmt.Sprintf("parametros invalidos: epocas 1..2000, tasa (0, 10], nodos 1..%d", len(s.cluster.Dirs)), http.StatusBadRequest)
		return
	}
	if !s.entrenando.TryLock() {
		fallo(w, "ya hay un entrenamiento en curso", http.StatusConflict)
		return
	}
	defer s.entrenando.Unlock()

	log.Printf("entrenamiento: %d epocas, tasa %.3f, %d nodos (pedido por %s)", in.Epocas, in.Tasa, in.Nodos, c.Subject)
	res, err := s.cluster.Entrenar(in.Nodos, s.cb.Dimension(), in.Epocas, in.Tasa, func(e int, p float64) {
		if e == 1 || e%25 == 0 || e == in.Epocas {
			log.Printf("  epoca %4d  perdida %.6f", e, p)
		}
	})
	if err != nil {
		fallo(w, "el cluster no pudo entrenar: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	_, actual := s.vigente()
	version := actual + 1
	m := &comun.Modelo{Columnas: s.cb.Nombres(), Pesos: res.Pesos, Codebook: s.cb, Workers: in.Nodos, Epocas: in.Epocas,
		Tasa: in.Tasa, FilasTrain: res.Filas, PerdidaFinal: res.Perdidas[len(res.Perdidas)-1], OddsRatios: map[string]float64{}}
	for j, n := range m.Columnas {
		if j > 0 {
			m.OddsRatios[n] = math.Exp(m.Pesos[j])
		}
	}
	if s.test != nil {
		prob := comun.Predecir(s.test.X, m.Pesos, 4)
		m.FilasTest = len(s.test.X)
		m.Metricas05, m.MetricasF1 = comun.Evaluar(s.test.Y, prob, 0.5), comun.MejorUmbral(s.test.Y, prob)
	}
	s.activar(m, version, "entrenamiento")
	s.store.GuardarModelo(ModeloDoc{Version: version, Fecha: time.Now(), Modelo: *m})
	s.store.GuardarModeloRedis(m, version)
	e := Entrenamiento{Version: version, Fecha: time.Now(), Usuario: c.Subject, Epocas: in.Epocas, Tasa: in.Tasa,
		NodosUsados: in.Nodos, Filas: res.Filas, Ms: res.Ms, MsPorEpoca: redondear(res.Ms / float64(in.Epocas)),
		Reasignaciones: res.Reasignaciones, PerdidaFinal: m.PerdidaFinal, AUC: m.Metricas05.AUC, F1: m.MetricasF1.F1,
		Nodos: res.Nodos, Perdidas: res.Perdidas}
	s.store.GuardarEntrenamiento(e)
	s.metricas.Entrenamiento()
	log.Printf("entrenamiento v%d: %.0f ms, perdida %.6f, AUC %.4f, reasignaciones %d", version, res.Ms, e.PerdidaFinal, e.AUC, e.Reasignaciones)
	e.Perdidas = nil
	responder(w, http.StatusOK, e)
}

// ---------- consultas

func (s *Servidor) verModelo(w http.ResponseWriter, r *http.Request) {
	m, v := s.vigente()
	s.mu.RLock()
	origen := s.origen
	s.mu.RUnlock()
	responder(w, http.StatusOK, map[string]any{"version": v, "origen": origen, "columnas": m.Columnas, "pesos": m.Pesos,
		"odds_ratios": m.OddsRatios, "metricas_umbral_0.5": m.Metricas05, "metricas_mejor_f1": m.MetricasF1,
		"epocas": m.Epocas, "filas_entrenamiento": m.FilasTrain, "filas_prueba": m.FilasTest,
		"categorias": map[string]any{"sexo": s.cb.Sexo, "dosis_previas": []int{0, 1, 2, 3}, "fabricante": s.cb.Fabricante,
			"departamento": s.cb.Departamento, "periodo": s.cb.Periodo, "edad": []int{s.cb.EdadMin, s.cb.EdadMax}}})
}

func (s *Servidor) verCluster(w http.ResponseWriter, r *http.Request) {
	nodos := s.cluster.Estado()
	activos := 0
	for _, n := range nodos {
		if n.Activo {
			activos++
		}
	}
	responder(w, http.StatusOK, map[string]any{"topologia": "estrella (parameter server sincrono)", "coordinador": "api",
		"protocolo": "TCP, un JSON por linea", "nodos_activos": activos, "nodos": nodos})
}

func (s *Servidor) verMetricas(w http.ResponseWriter, r *http.Request) {
	res := s.metricas.Resumen()
	res["mongodb"] = s.store.Conteos()
	if info := s.store.InfoRedis(); info != nil {
		res["redis"] = info
	}
	responder(w, http.StatusOK, res)
}

func limite(r *http.Request, defecto int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limite"))
	if err != nil || n < 1 || n > 200 {
		return defecto
	}
	return n
}

func (s *Servidor) verEntrenamientos(w http.ResponseWriter, r *http.Request, c *Claims) {
	responder(w, http.StatusOK, s.store.Entrenamientos(limite(r, 20)))
}

func (s *Servidor) verPredicciones(w http.ResponseWriter, r *http.Request, c *Claims) {
	responder(w, http.StatusOK, s.store.Predicciones(limite(r, 20)))
}

func (s *Servidor) openapi(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Write(especificacion)
}

func (s *Servidor) docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><title>API COVID hospitalizados</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui.css"></head><body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: "/openapi.yaml", dom_id: "#swagger-ui", persistAuthorization: true});</script></body></html>`)
}
