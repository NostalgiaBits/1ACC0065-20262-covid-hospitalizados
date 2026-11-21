# Reporte de participación — Entregable 2 (PC4)

**Curso:** CC65 Programación Concurrente y Distribuida · **Ciclo:** 2026-20 · **Grupo:** Gómez Rubina, Olivera Álvarez, Zegarra Garcia
**Coordinador:** Luis David Gómez Rubina · **Periodo:** 08/11/2026 – 21/11/2026

## 1. Reparto de responsabilidades

| Integrante | Código | Responsabilidad en PC4 | Ramas / evidencia en GitHub |
|---|---|---|---|
| Zegarra Garcia, Luis Enrique | U202123273 | Código en Go y Docker: clúster TCP (nodos y coordinador en estrella con reasignación de nodos caídos), API REST (predicción, entrenamiento distribuido, JWT, MongoDB, Redis, Swagger), Dockerfile y docker-compose, pruebas y benchmarks | `feature/cluster-tcp`, `feature/api-rest`, `feature/docker-compose` (issues #18–#23) |
| Gómez Rubina, Luis David | U20221C621 | Colección de Postman con pruebas automáticas y sus evidencias; reporte de participación; Kanban, integración y entrega | `feature/postman-pc4`, `feature/participacion-pc4` (issues #24, #26, #27) |
| Olivera Álvarez, Lizbeth Teresita | U201616851 | README del Entregable 2 (arquitectura, cómo levantar con Docker, API, Postman, resultados) y revisión del informe | `feature/readme-pc4` (issue #25) |

## 2. Registro de actividades

| Fecha | Integrante | Actividad | Horas | Estado |
|---|---|---|---|---|
| 20/11 | Luis Enrique | Protocolo TCP, coordinador del clúster, nodo ML y lanzador; pruebas de equivalencia y de nodo caído | | Hecho |
| 20/11 | Luis Enrique | API REST: predicción con caché, entrenamiento distribuido, JWT y bcrypt, MongoDB, Redis, OpenAPI | | Hecho |
| 20/11 | Luis Enrique | Dockerfile, docker-compose (3 nodos, API, MongoDB, Redis), benchmark por nodos y prueba de carga | | Hecho |
| 21/11 | Luis David | Colección de Postman, ejecución en Docker y capturas (Runner con 50 iteraciones) | Hecho |
| 21/11 | Lizbeth | README del Entregable 2 | Hecho |
| 21/11 | Todos | Revisión del informe, PR `develop → main`, release `pc4` y entrega | Hecho |

## 3. Reuniones

| Fecha | Medio | Asistentes | Acuerdos |
|---|---|---|---|
|15/11 |Discord | Todos los integrantes| Topología en estrella (parameter server); Docker Compose con IPs fijas como en la clase de la semana 12 |
| 19/11|Google Meets |Todos los integrantes | Reparto: código Go y Docker (Luis Enrique), Postman y gestión (Luis David), README (Lizbeth) |

## 4. Autoevaluación (1–5)

| Criterio | Luis Enrique | Luis David | Lizbeth |
|---|---|---|---|
| Cumplimiento de las tareas asignadas |20 |20 |20 |
| Puntualidad en las entregas internas |20 |20 |20 |
| Comunicación con el equipo |20 |20 |20 |
| Calidad del trabajo entregado |20 |20 |20 |

