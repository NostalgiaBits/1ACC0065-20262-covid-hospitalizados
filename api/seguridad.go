package main

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	Rol string `json:"rol"`
	jwt.RegisteredClaims
}

const duracionToken = time.Hour

func (s *Servidor) emitirToken(u Usuario) (string, time.Time, error) {
	expira := time.Now().Add(duracionToken)
	claims := Claims{Rol: u.Rol, RegisteredClaims: jwt.RegisteredClaims{
		Subject: u.Email, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(expira), Issuer: "covid-api",
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secreto)
	return token, expira, err
}

func (s *Servidor) validarToken(texto string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(texto, claims, func(t *jwt.Token) (any, error) {
		return s.secreto, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("covid-api"))
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// exige "Authorization: Bearer <token>"; rol vacio = cualquier usuario autenticado
func (s *Servidor) protegido(rol string, h func(http.ResponseWriter, *http.Request, *Claims)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		texto, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || texto == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			fallo(w, "falta el token: Authorization: Bearer <token>", http.StatusUnauthorized)
			return
		}
		claims, err := s.validarToken(texto)
		if err != nil {
			fallo(w, "token invalido o expirado", http.StatusUnauthorized)
			return
		}
		if rol != "" && claims.Rol != rol {
			fallo(w, "se requiere el rol "+rol, http.StatusForbidden)
			return
		}
		h(w, r, claims)
	}
}

func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(h), err
}

func passwordCorrecta(hash, p string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil
}

func (s *Servidor) sembrarAdmin(email, password string) {
	if email == "" || password == "" {
		log.Println("AVISO: ADMIN_EMAIL/ADMIN_PASSWORD no definidos; no hay usuario administrador")
		return
	}
	hash, err := hashPassword(password)
	if err != nil {
		log.Println("admin:", err)
		return
	}
	u := Usuario{Email: strings.ToLower(email), Nombre: "Administrador", Hash: hash, Rol: "admin", Creado: time.Now()}
	if err := s.store.GuardarUsuario(u); err != nil {
		log.Println("admin:", err)
	}
}

var errCredenciales = errors.New("credenciales incorrectas")

func (s *Servidor) autenticar(email, password string) (Usuario, error) {
	u, ok := s.store.BuscarUsuario(strings.ToLower(email))
	if !ok || !passwordCorrecta(u.Hash, password) {
		return Usuario{}, errCredenciales
	}
	return u, nil
}
