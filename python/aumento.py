"""Separacion por persona y aumento del conjunto de entrenamiento a mas de 1 000 000 de registros.

1. Divide el dataset limpio en entrenamiento (80 %) y prueba (20 %) POR PERSONA (id_persona), para que una misma
   persona con varias hospitalizaciones no quede en los dos lados.
2. El conjunto de prueba se guarda tal cual (solo filas reales): las metricas se calculan siempre sobre datos reales.
3. El conjunto de entrenamiento se aumenta por bootstrap estratificado por clase (misma proporcion de fallecidos) con ruido
   controlado en la edad (+-2 anios). Las filas sinteticas llevan id_persona negativo.

Entrada : data/hospitalizados_limpio.csv, data/codebook.json
Salida  : data/train_1M.csv, data/test.csv, data/muestra_1000.csv, codebook con media/desv de edad, data/aumento_resumen.json
Uso: python python/aumento.py [--target 1050000] [--seed 42]
"""
import argparse
import json

import numpy as np
import pandas as pd

ap = argparse.ArgumentParser()
ap.add_argument("--input", default="data/hospitalizados_limpio.csv")
ap.add_argument("--codebook", default="data/codebook.json")
ap.add_argument("--train", default="data/train_1M.csv")
ap.add_argument("--test", default="data/test.csv")
ap.add_argument("--muestra", default="data/muestra_1000.csv")
ap.add_argument("--resumen", default="data/aumento_resumen.json")
ap.add_argument("--target", type=int, default=1_050_000)
ap.add_argument("--test-frac", type=float, default=0.20)
ap.add_argument("--seed", type=int, default=42)
args = ap.parse_args()

rng = np.random.default_rng(args.seed)
df = pd.read_csv(args.input)
resumen = {"filas_reales": len(df), "seed": args.seed}

# ---- 1. split por persona
personas = df["id_persona"].unique()
rng.shuffle(personas)
n_test = int(round(len(personas) * args.test_frac))
personas_test = set(personas[:n_test].tolist())
es_test = df["id_persona"].isin(personas_test)
test = df[es_test].reset_index(drop=True)
train_real = df[~es_test].reset_index(drop=True)
resumen["personas"] = int(len(personas))
resumen["train_real"] = {"filas": len(train_real), "fallecidos": int(train_real["fallecido"].sum()),
                         "tasa": round(float(train_real["fallecido"].mean()), 4)}
resumen["test"] = {"filas": len(test), "fallecidos": int(test["fallecido"].sum()),
                   "tasa": round(float(test["fallecido"].mean()), 4)}
print(f"1. Split por persona: train real {len(train_real)} filas, test {len(test)} filas (tasa fallecidos "
      f"{resumen['train_real']['tasa']:.1%} / {resumen['test']['tasa']:.1%})")

# ---- 2. media y desviacion de la edad del entrenamiento real (Go estandariza con estos valores)
with open(args.codebook, encoding="utf-8") as f:
    codebook = json.load(f)
codebook["edad_media"] = round(float(train_real["edad"].mean()), 4)
codebook["edad_desv"] = round(float(train_real["edad"].std(ddof=0)), 4)
with open(args.codebook, "w", encoding="utf-8") as f:
    json.dump(codebook, f, ensure_ascii=False, indent=2)
print(f"2. Edad del train real: media {codebook['edad_media']}, desv {codebook['edad_desv']} -> codebook")

# ---- 3. aumento: bootstrap estratificado por clase hasta llegar a --target filas en total
faltan = args.target - len(train_real)
tasa = train_real["fallecido"].mean()
n_pos = int(round(faltan * tasa))
n_neg = faltan - n_pos
partes = [train_real]
for clase, n in ((1, n_pos), (0, n_neg)):
    base = train_real[train_real["fallecido"] == clase]
    idx = rng.integers(0, len(base), size=n)
    sint = base.iloc[idx].copy().reset_index(drop=True)
    sint["edad"] = np.clip(sint["edad"].values + rng.integers(-2, 3, size=n), codebook["edad_min"], codebook["edad_max"])
    partes.append(sint)
train = pd.concat(partes, ignore_index=True)
n_sint = len(train) - len(train_real)
train.loc[len(train_real):, "id_persona"] = -np.arange(1, n_sint + 1)
train = train.sample(frac=1.0, random_state=args.seed).reset_index(drop=True)
resumen["train_aumentado"] = {"filas": len(train), "sinteticas": int(n_sint), "reales": len(train_real),
                              "fallecidos": int(train["fallecido"].sum()), "tasa": round(float(train["fallecido"].mean()), 4),
                              "ruido": "edad +-2 anios; resto de columnas intactas"}
print(f"3. Train aumentado: {len(train)} filas ({len(train_real)} reales + {n_sint} sinteticas), tasa {resumen['train_aumentado']['tasa']:.1%}")

# ---- 4. salidas
train.to_csv(args.train, index=False)
test.to_csv(args.test, index=False)
train.head(1000).to_csv(args.muestra, index=False)
with open(args.resumen, "w", encoding="utf-8") as f:
    json.dump(resumen, f, ensure_ascii=False, indent=2)
print(f"4. -> {args.train}, {args.test}, {args.muestra}, {args.resumen}")
