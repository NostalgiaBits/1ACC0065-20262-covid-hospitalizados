# Imagen base: la version de Go que pide go.mod
FROM golang:1.26
# Carpeta de trabajo en el contenedor
WORKDIR /app
# Dependencias primero: esta capa se reutiliza mientras go.mod y go.sum no cambien
COPY go.mod go.sum ./
RUN go mod download
# Codigo del proyecto
COPY . .
# Programa a compilar: api o nodo
ARG PROGRAMA=api
RUN go build -o /app/servicio ./${PROGRAMA}
# Puertos: 8080 la API, 9091 los nodos
EXPOSE 8080 9091
# Punto de ejecucion: el binario ya compilado
CMD ["/app/servicio"]
