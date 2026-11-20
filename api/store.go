package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"covid-hospitalizados/comun"
)

// documentos de MongoDB
type Usuario struct {
	Email  string    `json:"email" bson:"email"`
	Nombre string    `json:"nombre" bson:"nombre"`
	Hash   string    `json:"-" bson:"hash"`
	Rol    string    `json:"rol" bson:"rol"`
	Creado time.Time `json:"creado" bson:"creado"`
}

type ModeloDoc struct {
	Version int          `json:"version" bson:"version"`
	Fecha   time.Time    `json:"fecha" bson:"fecha"`
	Modelo  comun.Modelo `json:"modelo" bson:"modelo"`
}

type Entrenamiento struct {
	Version        int                `json:"version" bson:"version"`
	Fecha          time.Time          `json:"fecha" bson:"fecha"`
	Usuario        string             `json:"usuario" bson:"usuario"`
	Epocas         int                `json:"epocas" bson:"epocas"`
	Tasa           float64            `json:"tasa" bson:"tasa"`
	NodosUsados    int                `json:"nodos_usados" bson:"nodos_usados"`
	Filas          int                `json:"filas" bson:"filas"`
	Ms             float64            `json:"ms" bson:"ms"`
	MsPorEpoca     float64            `json:"ms_por_epoca" bson:"ms_por_epoca"`
	Reasignaciones int                `json:"reasignaciones" bson:"reasignaciones"`
	PerdidaFinal   float64            `json:"perdida_final" bson:"perdida_final"`
	AUC            float64            `json:"auc" bson:"auc"`
	F1             float64            `json:"f1" bson:"f1"`
	Nodos          []comun.EstadoNodo `json:"nodos" bson:"nodos"`
	Perdidas       []float64          `json:"perdidas,omitempty" bson:"perdidas"`
}

type Prediccion struct {
	Fecha         time.Time      `json:"fecha" bson:"fecha"`
	Entrada       comun.Paciente `json:"entrada" bson:"entrada"`
	Probabilidad  float64        `json:"probabilidad" bson:"probabilidad"`
	Riesgo        string         `json:"riesgo" bson:"riesgo"`
	ModeloVersion int            `json:"modelo_version" bson:"modelo_version"`
	LatenciaMs    float64        `json:"latencia_ms" bson:"latencia_ms"`
	Cache         bool           `json:"cache" bson:"cache"`
}

// Mongo y Redis son opcionales: sin ellos la API guarda en memoria (para probar sin Docker)
type Store struct {
	Mongo *mongo.Database
	Redis *redis.Client

	mu             sync.Mutex
	usuarios       map[string]Usuario
	entrenamientos []Entrenamiento
	predicciones   []Prediccion
	modelos        []ModeloDoc
}

func ctx5() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func ConectarStore(mongoURI, base, redisAddr string) *Store {
	s := &Store{usuarios: map[string]Usuario{}}
	if mongoURI != "" {
		for intento := 1; intento <= 10; intento++ {
			cliente, err := mongo.Connect(options.Client().ApplyURI(mongoURI).SetServerSelectionTimeout(3 * time.Second))
			if err == nil {
				ctx, cancel := ctx5()
				err = cliente.Ping(ctx, nil)
				cancel()
			}
			if err == nil {
				s.Mongo = cliente.Database(base)
				s.crearIndices()
				break
			}
			log.Printf("mongo no disponible (intento %d): %v", intento, err)
			time.Sleep(2 * time.Second)
		}
	}
	if redisAddr != "" {
		cliente := redis.NewClient(&redis.Options{Addr: redisAddr})
		for intento := 1; intento <= 10; intento++ {
			ctx, cancel := ctx5()
			err := cliente.Ping(ctx).Err()
			cancel()
			if err == nil {
				s.Redis = cliente
				break
			}
			log.Printf("redis no disponible (intento %d): %v", intento, err)
			time.Sleep(2 * time.Second)
		}
	}
	return s
}

func (s *Store) crearIndices() {
	ctx, cancel := ctx5()
	defer cancel()
	s.Mongo.Collection("usuarios").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true)})
	s.Mongo.Collection("modelos").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "version", Value: -1}}})
	s.Mongo.Collection("predicciones").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "fecha", Value: -1}}})
	s.Mongo.Collection("entrenamientos").Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "fecha", Value: -1}}})
}

var ErrExiste = errors.New("ya existe")

// ---------- usuarios

func (s *Store) CrearUsuario(u Usuario) error {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		_, err := s.Mongo.Collection("usuarios").InsertOne(ctx, u)
		if mongo.IsDuplicateKeyError(err) {
			return ErrExiste
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usuarios[u.Email]; ok {
		return ErrExiste
	}
	s.usuarios[u.Email] = u
	return nil
}

func (s *Store) GuardarUsuario(u Usuario) error {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		_, err := s.Mongo.Collection("usuarios").ReplaceOne(ctx, bson.M{"email": u.Email}, u, options.Replace().SetUpsert(true))
		return err
	}
	s.mu.Lock()
	s.usuarios[u.Email] = u
	s.mu.Unlock()
	return nil
}

func (s *Store) BuscarUsuario(email string) (Usuario, bool) {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		var u Usuario
		err := s.Mongo.Collection("usuarios").FindOne(ctx, bson.M{"email": email}).Decode(&u)
		return u, err == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.usuarios[email]
	return u, ok
}

// ---------- modelos

func (s *Store) GuardarModelo(doc ModeloDoc) {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		if _, err := s.Mongo.Collection("modelos").InsertOne(ctx, doc); err != nil {
			log.Println("mongo modelos:", err)
		}
		return
	}
	s.mu.Lock()
	s.modelos = append(s.modelos, doc)
	s.mu.Unlock()
}

func (s *Store) UltimoModelo() (*comun.Modelo, int, bool) {
	if s.Mongo == nil {
		return nil, 0, false
	}
	ctx, cancel := ctx5()
	defer cancel()
	var doc ModeloDoc
	opts := options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})
	if err := s.Mongo.Collection("modelos").FindOne(ctx, bson.D{}, opts).Decode(&doc); err != nil {
		return nil, 0, false
	}
	return &doc.Modelo, doc.Version, true
}

func (s *Store) GuardarModeloRedis(m *comun.Modelo, version int) {
	if s.Redis == nil {
		return
	}
	ctx, cancel := ctx5()
	defer cancel()
	datos, _ := json.Marshal(m)
	s.Redis.Set(ctx, "modelo:actual", datos, 0)
	s.Redis.Set(ctx, "modelo:version", version, 0)
}

func (s *Store) ModeloRedis() (*comun.Modelo, int, bool) {
	if s.Redis == nil {
		return nil, 0, false
	}
	ctx, cancel := ctx5()
	defer cancel()
	datos, err := s.Redis.Get(ctx, "modelo:actual").Bytes()
	if err != nil {
		return nil, 0, false
	}
	v, _ := s.Redis.Get(ctx, "modelo:version").Int()
	var m comun.Modelo
	if json.Unmarshal(datos, &m) != nil {
		return nil, 0, false
	}
	return &m, v, true
}

// ---------- entrenamientos y predicciones

func (s *Store) GuardarEntrenamiento(e Entrenamiento) {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		if _, err := s.Mongo.Collection("entrenamientos").InsertOne(ctx, e); err != nil {
			log.Println("mongo entrenamientos:", err)
		}
		return
	}
	s.mu.Lock()
	s.entrenamientos = append(s.entrenamientos, e)
	s.mu.Unlock()
}

func (s *Store) Entrenamientos(limite int) []Entrenamiento {
	res := []Entrenamiento{}
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: -1}}).SetLimit(int64(limite)).SetProjection(bson.M{"perdidas": 0})
		if cur, err := s.Mongo.Collection("entrenamientos").Find(ctx, bson.D{}, opts); err == nil {
			cur.All(ctx, &res)
		}
		return res
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.entrenamientos) - 1; i >= 0 && len(res) < limite; i-- {
		e := s.entrenamientos[i]
		e.Perdidas = nil
		res = append(res, e)
	}
	return res
}

func (s *Store) GuardarPrediccion(p Prediccion) {
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		if _, err := s.Mongo.Collection("predicciones").InsertOne(ctx, p); err != nil {
			log.Println("mongo predicciones:", err)
		}
		return
	}
	s.mu.Lock()
	s.predicciones = append(s.predicciones, p)
	if len(s.predicciones) > 1000 {
		s.predicciones = s.predicciones[len(s.predicciones)-1000:]
	}
	s.mu.Unlock()
}

func (s *Store) Predicciones(limite int) []Prediccion {
	res := []Prediccion{}
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: -1}}).SetLimit(int64(limite))
		if cur, err := s.Mongo.Collection("predicciones").Find(ctx, bson.D{}, opts); err == nil {
			cur.All(ctx, &res)
		}
		return res
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.predicciones) - 1; i >= 0 && len(res) < limite; i-- {
		res = append(res, s.predicciones[i])
	}
	return res
}

func (s *Store) Conteos() map[string]int64 {
	c := map[string]int64{}
	if s.Mongo != nil {
		ctx, cancel := ctx5()
		defer cancel()
		for _, col := range []string{"usuarios", "modelos", "entrenamientos", "predicciones"} {
			n, _ := s.Mongo.Collection(col).CountDocuments(ctx, bson.D{})
			c[col] = n
		}
		return c
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c["usuarios"], c["modelos"] = int64(len(s.usuarios)), int64(len(s.modelos))
	c["entrenamientos"], c["predicciones"] = int64(len(s.entrenamientos)), int64(len(s.predicciones))
	return c
}

// ---------- cache de predicciones en Redis

func (s *Store) CacheGet(clave string) ([]byte, bool) {
	if s.Redis == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	datos, err := s.Redis.Get(ctx, clave).Bytes()
	return datos, err == nil
}

func (s *Store) CacheSet(clave string, datos []byte) {
	if s.Redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	s.Redis.Set(ctx, clave, datos, time.Hour)
}

func (s *Store) RegistrarLatencia(ms float64, hit bool) {
	if s.Redis == nil {
		return
	}
	ctx, cancel := ctx5()
	defer cancel()
	pipe := s.Redis.Pipeline()
	pipe.LPush(ctx, "metricas:latencias", strconv.FormatFloat(ms, 'f', 3, 64))
	pipe.LTrim(ctx, "metricas:latencias", 0, 999)
	pipe.Incr(ctx, "metricas:predicciones")
	if hit {
		pipe.Incr(ctx, "metricas:cache_hits")
	}
	pipe.Exec(ctx)
}

func (s *Store) InfoRedis() map[string]any {
	if s.Redis == nil {
		return nil
	}
	ctx, cancel := ctx5()
	defer cancel()
	pred, _ := s.Redis.Get(ctx, "metricas:predicciones").Int64()
	hits, _ := s.Redis.Get(ctx, "metricas:cache_hits").Int64()
	claves, _ := s.Redis.DBSize(ctx).Result()
	cache := 0
	iter := s.Redis.Scan(ctx, 0, "pred:*", 1000).Iterator()
	for iter.Next(ctx) {
		cache++
	}
	return map[string]any{"predicciones": pred, "cache_hits": hits, "claves": claves, "predicciones_en_cache": cache}
}

func (s *Store) Estado() (bool, bool) {
	mongoOK, redisOK := false, false
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if s.Mongo != nil {
		mongoOK = s.Mongo.Client().Ping(ctx, nil) == nil
	}
	if s.Redis != nil {
		redisOK = s.Redis.Ping(ctx).Err() == nil
	}
	return mongoOK, redisOK
}
