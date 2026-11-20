package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"covid-hospitalizados/comun"
)

func servidorPrueba(t *testing.T) *Servidor {
	t.Helper()
	cb, err := comun.CargarCodebook("../data/codebook.json")
	if err != nil {
		t.Fatal(err)
	}
	datos, err := comun.CargarCSV("../data/muestra_1000.csv", cb, 2)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for i := 0; i < 2; i++ {
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
				go comun.AtenderNodo(conn, datos, "prueba", 2)
			}
		}()
		dirs = append(dirs, ln.Addr().String())
	}
	s := &Servidor{cb: cb, metricas: NuevasMetricas(), secreto: []byte("secreto-de-prueba"), test: datos,
		cluster: comun.NuevoCluster(dirs, 5*time.Second), store: ConectarStore("", "", "")}
	contenido, _ := os.ReadFile("../data/modelo.json")
	var m comun.Modelo
	if err := json.Unmarshal(contenido, &m); err != nil {
		t.Fatal(err)
	}
	s.activar(&m, 1, "archivo")
	s.sembrarAdmin("admin@prueba.pe", "admin-12345")
	return s
}

func pedir(s *Servidor, metodo, ruta, token string, cuerpo any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if cuerpo != nil {
		json.NewEncoder(&buf).Encode(cuerpo)
	}
	r := httptest.NewRequest(metodo, ruta, &buf)
	if cuerpo != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

var paciente = map[string]any{"edad": 78, "sexo": "M", "dosis_previas": 0, "fabricante": "NINGUNA", "departamento": "LIMA", "periodo": "2021-S1"}

func TestPredecir(t *testing.T) {
	s := servidorPrueba(t)
	w := pedir(s, "POST", "/predecir", "", paciente)
	if w.Code != 200 {
		t.Fatalf("codigo %d: %s", w.Code, w.Body)
	}
	var res respuestaPrediccion
	json.Unmarshal(w.Body.Bytes(), &res)
	if res.Probabilidad <= 0 || res.Probabilidad >= 1 || res.Riesgo == "" || res.LatenciaMs > 100 || len(res.Factores) == 0 {
		t.Errorf("respuesta inesperada: %+v", res)
	}
	joven := map[string]any{"edad": 25, "sexo": "F", "dosis_previas": 3, "fabricante": "PFIZER", "departamento": "LIMA", "periodo": "2022+"}
	var res2 respuestaPrediccion
	json.Unmarshal(pedir(s, "POST", "/predecir", "", joven).Body.Bytes(), &res2)
	if res2.Probabilidad >= res.Probabilidad {
		t.Errorf("una joven vacunada en 2022 deberia tener menos riesgo: %v vs %v", res2.Probabilidad, res.Probabilidad)
	}
}

func TestErroresHTTP(t *testing.T) {
	s := servidorPrueba(t)
	casos := []struct {
		metodo, ruta string
		cuerpo       any
		tipo         string
		codigo       int
	}{
		{"GET", "/predecir", nil, "", 405},
		{"POST", "/predecir", paciente, "text/plain", 415},
		{"POST", "/predecir", map[string]any{"edad": 50}, "application/json", 400},
		{"POST", "/predecir", map[string]any{"edad": 50, "sexo": "M", "dosis_previas": 0, "departamento": "MARTE", "periodo": "2021-S1"}, "application/json", 400},
		{"POST", "/predecir", map[string]any{"edad": 500, "sexo": "M", "dosis_previas": 0, "departamento": "LIMA", "periodo": "2021-S1"}, "application/json", 400},
		{"POST", "/predecir", map[string]any{"campo": 1}, "application/json", 400},
		{"GET", "/no-existe", nil, "", 404},
	}
	for _, c := range casos {
		var buf bytes.Buffer
		if c.cuerpo != nil {
			json.NewEncoder(&buf).Encode(c.cuerpo)
		}
		r := httptest.NewRequest(c.metodo, c.ruta, &buf)
		if c.tipo != "" {
			r.Header.Set("Content-Type", c.tipo)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != c.codigo {
			t.Errorf("%s %s (%s): codigo %d, esperado %d: %s", c.metodo, c.ruta, c.tipo, w.Code, c.codigo, w.Body)
		}
	}
}

func token(t *testing.T, s *Servidor, email, password string) string {
	w := pedir(s, "POST", "/login", "", map[string]string{"email": email, "password": password})
	if w.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, w.Code, w.Body)
	}
	var res map[string]any
	json.Unmarshal(w.Body.Bytes(), &res)
	return res["token"].(string)
}

func TestUsuariosYEntrenamiento(t *testing.T) {
	s := servidorPrueba(t)
	nuevo := map[string]string{"email": "medico@hospital.pe", "nombre": "Medico", "password": "clave-segura-1"}
	if w := pedir(s, "POST", "/registro", "", nuevo); w.Code != 201 {
		t.Fatalf("registro: %d %s", w.Code, w.Body)
	}
	if w := pedir(s, "POST", "/registro", "", nuevo); w.Code != 409 {
		t.Errorf("registro duplicado: %d", w.Code)
	}
	if w := pedir(s, "POST", "/login", "", map[string]string{"email": "medico@hospital.pe", "password": "otra"}); w.Code != 401 {
		t.Errorf("login con clave incorrecta: %d", w.Code)
	}
	medico := token(t, s, "medico@hospital.pe", "clave-segura-1")
	cuerpo := map[string]any{"epocas": 5, "tasa": 1.0, "nodos": 2}
	if w := pedir(s, "POST", "/entrenar", "", cuerpo); w.Code != 401 {
		t.Errorf("entrenar sin token: %d", w.Code)
	}
	if w := pedir(s, "POST", "/entrenar", "token-falso", cuerpo); w.Code != 401 {
		t.Errorf("entrenar con token falso: %d", w.Code)
	}
	if w := pedir(s, "POST", "/entrenar", medico, cuerpo); w.Code != 403 {
		t.Errorf("entrenar sin rol admin: %d", w.Code)
	}
	admin := token(t, s, "admin@prueba.pe", "admin-12345")
	w := pedir(s, "POST", "/entrenar", admin, cuerpo)
	if w.Code != 200 {
		t.Fatalf("entrenar: %d %s", w.Code, w.Body)
	}
	var e Entrenamiento
	json.Unmarshal(w.Body.Bytes(), &e)
	if e.Version != 2 || e.NodosUsados != 2 || e.Reasignaciones != 0 || e.AUC <= 0.5 || e.Nodos[0].Peticiones != 5 {
		t.Errorf("entrenamiento inesperado: %+v", e)
	}
	if _, v := s.vigente(); v != 2 {
		t.Errorf("el modelo nuevo no se activo (version %d)", v)
	}
	if w := pedir(s, "GET", "/entrenamientos", medico, nil); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"version":2`)) {
		t.Errorf("historial: %d %s", w.Code, w.Body)
	}
}
