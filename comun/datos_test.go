package comun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codebookPrueba() *Codebook {
	cb := &Codebook{
		Sexo: []string{"F", "M"}, Dosis: []string{"0", "1", "2", "3+"},
		Fabricante:   []string{"NINGUNA", "PFIZER", "SINOPHARM", "ASTRAZENECA", "OTRO"},
		Departamento: []string{"LIMA", "AREQUIPA", "CUSCO"}, Periodo: []string{"2020-S1", "2020-S2", "2021-S1", "2021-S2", "2022+"},
		EdadMin: 0, EdadMax: 110, EdadMedia: 50, EdadDesv: 20,
	}
	cb.pos = map[string]int{}
	for i, n := range cb.Nombres() {
		cb.pos[n] = i
	}
	return cb
}

func TestCodificar(t *testing.T) {
	cb := codebookPrueba()
	x := make([]float64, cb.Dimension())
	y, err := cb.Codificar("123,70,M,2,PFIZER,CUSCO,2021-S1,1", x)
	if err != nil || y != 1 {
		t.Fatalf("linea valida rechazada: %v", err)
	}
	if x[0] != 1 || x[1] != 1.0 { // (70-50)/20
		t.Errorf("sesgo/edad_z incorrectos: %v", x[:2])
	}
	esperado := map[string]bool{"sexo=M": true, "dosis=2": true, "fabricante=PFIZER": true, "dep=CUSCO": true, "periodo=2021-S1": true}
	for i, n := range cb.Nombres()[2:] {
		if (x[i+2] == 1) != esperado[n] {
			t.Errorf("columna %s = %v", n, x[i+2])
		}
	}
	if _, err := cb.Codificar("1,50,F,0,NINGUNA,LIMA,2020-S1,0", x); err != nil {
		t.Fatal(err)
	}
	for i := 2; i < len(x); i++ {
		if x[i] != 0 {
			t.Errorf("categoria de referencia encendio %s", cb.Nombres()[i])
		}
	}
	for _, mala := range []string{"1,50,F,0,NINGUNA,LIMA,2020-S1", "1,abc,F,0,NINGUNA,LIMA,2020-S1,0", "1,150,F,0,NINGUNA,LIMA,2020-S1,0",
		"1,50,X,0,NINGUNA,LIMA,2020-S1,0", "1,50,F,9,NINGUNA,LIMA,2020-S1,0", "1,50,F,0,NINGUNA,MARTE,2020-S1,0", "1,50,F,0,NINGUNA,LIMA,2020-S1,2"} {
		if _, err := cb.Codificar(mala, x); err == nil {
			t.Errorf("linea invalida aceptada: %s", mala)
		}
	}
}

// bloques pequenos: ninguna linea se parte y se conserva el orden
func TestCargarCSVBloquesNoParteLineas(t *testing.T) {
	cb := codebookPrueba()
	ruta := filepath.Join(t.TempDir(), "datos.csv")
	var sb strings.Builder
	sb.WriteString("id_persona,edad,sexo,dosis_previas,fabricante,departamento,periodo,fallecido\r\n")
	n := 1000
	for i := 0; i < n; i++ {
		dep := []string{"LIMA", "AREQUIPA", "CUSCO"}[i%3]
		fmt.Fprintf(&sb, "%d,%d,%s,%d,%s,%s,%s,%d\n", i, i%100, "FM"[i%2:i%2+1], i%4,
			[]string{"NINGUNA", "PFIZER", "SINOPHARM", "ASTRAZENECA", "OTRO"}[i%5], dep, "2021-S1", i%2)
	}
	sb.WriteString("9999,50,F,0,NINGUNA,LIMA,2020-S1,0")
	sb.WriteString("\n1,50,Z,0,NINGUNA,LIMA,2020-S1,0")
	if err := os.WriteFile(ruta, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
	for _, tam := range []int{7, 64, 1000, 1 << 20} {
		for _, workers := range []int{1, 3, 8} {
			d, err := CargarCSVBloques(ruta, cb, workers, tam)
			if err != nil {
				t.Fatal(err)
			}
			if d.Lineas != n+2 || len(d.X) != n+1 || d.Invalidas != 1 {
				t.Errorf("bloque %d workers %d: lineas %d validas %d invalidas %d", tam, workers, d.Lineas, len(d.X), d.Invalidas)
			}
			for i := 0; i < n; i++ {
				if d.X[i][1] != (float64(i%100)-50)/20 {
					t.Fatalf("bloque %d workers %d: fila %d fuera de orden", tam, workers, i)
				}
			}
		}
	}
}
