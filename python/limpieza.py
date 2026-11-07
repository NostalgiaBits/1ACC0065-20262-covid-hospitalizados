"""Limpieza del dataset "Hospitalizados, vacunados y fallecidos por COVID-19" (MINSA, datosabiertos.gob.pe).

Entrada : data/raw/TB_HOSP_VAC_FALLECIDOS.csv  (separador ';', UTF-8 con BOM, fechas dd/mm/aaaa)
Salida  : data/hospitalizados_limpio.csv  (una fila por hospitalizacion, 8 columnas)
          data/codebook.json               (categorias en orden fijo, para que Go codifique igual)
          data/limpieza_resumen.json       (conteos de cada paso, para el informe)

Uso: python python/limpieza.py [--input ...] [--output ...]
"""
import argparse
import json
import os

import numpy as np
import pandas as pd

DIAS_PROTECCION = 14  # una dosis cuenta si se aplico 14 dias o mas antes del ingreso

ap = argparse.ArgumentParser()
ap.add_argument("--input", default="data/raw/TB_HOSP_VAC_FALLECIDOS.csv")
ap.add_argument("--output", default="data/hospitalizados_limpio.csv")
ap.add_argument("--codebook", default="data/codebook.json")
ap.add_argument("--resumen", default="data/limpieza_resumen.json")
args = ap.parse_args()

resumen = {}
df = pd.read_csv(args.input, sep=";", encoding="utf-8-sig", dtype=str, keep_default_na=False)
resumen["filas_original"] = len(df)
resumen["columnas_original"] = df.shape[1]
print(f"1. Original: {len(df)} filas x {df.shape[1]} columnas")

# ---- 2. Variables que ocurren despues del ingreso (fuga de informacion) y sin informacion: se descartan
fuga = ["flag_uci", "fecha_ingreso_uci", "fecha_ingreso_ucin", "con_oxigeno", "con_ventilacion",
        "fecha_segumiento_hosp_ultimo", "evolucion_hosp_ultimo", "cdc_fecha_fallecido_covid", "flag_vacuna"]
sin_info = ["cdc_positividad", "eess_renaes", "eess_diresa", "eess_red", "eess_nombre", "id_eess",
            "ubigeo_inei_domicilio", "prov_domicilio", "dist_domicilio"]
resumen["columnas_fuga"] = fuga
resumen["columnas_sin_info"] = sin_info
print(f"2. Se descartan {len(fuga)} columnas posteriores al ingreso y {len(sin_info)} sin valor predictivo")

# ---- 3. Tipos
df["edad"] = pd.to_numeric(df["edad"], errors="coerce")
df["fecha_ingreso"] = pd.to_datetime(df["fecha_ingreso_hosp"], dayfirst=True, errors="coerce")
df["fallecido"] = pd.to_numeric(df["cdc_fallecido_covid"], errors="coerce")
for k in range(1, 7):
    df[f"fd{k}"] = pd.to_datetime(df[f"fecha_dosis{k}"], dayfirst=True, errors="coerce")

# ---- 4. Filas sin edad, sin sexo, sin fecha de ingreso o con edad fuera de rango
n0 = len(df)
sin_edad = df["edad"].isna().sum()
sin_sexo = (~df["sexo"].isin(["F", "M"])).sum()
sin_fecha = df["fecha_ingreso"].isna().sum()
df = df[df["edad"].notna() & df["sexo"].isin(["F", "M"]) & df["fecha_ingreso"].notna()]
df = df[(df["edad"] >= 0) & (df["edad"] <= 110)]
df = df[df["fallecido"].isin([0, 1])]
resumen["sin_edad"] = int(sin_edad)
resumen["sin_sexo"] = int(sin_sexo)
resumen["sin_fecha_ingreso"] = int(sin_fecha)
resumen["filas_eliminadas_nulos"] = int(n0 - len(df))
resumen["filas_tras_nulos"] = len(df)
print(f"4. Sin edad: {sin_edad}, sin sexo: {sin_sexo}, sin fecha: {sin_fecha} -> quedan {len(df)} filas")

# ---- 5. Dosis previas al ingreso (>= 14 dias antes). flag_vacuna NO sirve: cuenta dosis posteriores al ingreso.
previas = np.zeros(len(df), dtype=int)
primera_previa = pd.Series([pd.NaT] * len(df), index=df.index)
fabricante_previa = pd.Series([""] * len(df), index=df.index)
for k in range(1, 7):
    dias = (df["fecha_ingreso"] - df[f"fd{k}"]).dt.days
    es_previa = (dias >= DIAS_PROTECCION).fillna(False).values
    previas += es_previa.astype(int)
    nueva = es_previa & (fabricante_previa == "").values
    fabricante_previa[nueva] = df.loc[nueva, f"fabricante_dosis{k}"].str.strip().str.upper()
df["dosis_previas"] = np.minimum(previas, 3)  # 0, 1, 2, 3+
fab = fabricante_previa.replace("", "NINGUNA")
fab[(df["dosis_previas"] > 0) & (~fab.isin(["PFIZER", "SINOPHARM", "ASTRAZENECA"]))] = "OTRO"
df["fabricante"] = fab
resumen["dosis_previas"] = {str(k): int(v) for k, v in df["dosis_previas"].value_counts().sort_index().items()}
resumen["mortalidad_por_dosis_previas"] = {str(k): round(float(v), 4) for k, v in df.groupby("dosis_previas")["fallecido"].mean().items()}
print(f"5. Dosis previas (>= {DIAS_PROTECCION} dias): {resumen['dosis_previas']}")
print(f"   Mortalidad por dosis previas: {resumen['mortalidad_por_dosis_previas']}")

# ---- 6. Periodo de la pandemia (ola) a partir de la fecha de ingreso
def periodo(f):
    if f.year <= 2020:
        return "2020-S1" if f.month <= 6 else "2020-S2"
    if f.year == 2021:
        return "2021-S1" if f.month <= 6 else "2021-S2"
    return "2022+"
df["periodo"] = df["fecha_ingreso"].apply(periodo)
resumen["periodo"] = {k: int(v) for k, v in df["periodo"].value_counts().sort_index().items()}
resumen["mortalidad_por_periodo"] = {k: round(float(v), 4) for k, v in df.groupby("periodo")["fallecido"].mean().items()}
print(f"6. Periodo: {resumen['periodo']}")

# ---- 7. Departamento, edad entera, target
df["departamento"] = df["dep_domicilio"].str.strip().str.upper()
df["edad"] = df["edad"].astype(int)
df["fallecido"] = df["fallecido"].astype(int)
df["id_persona"] = pd.to_numeric(df["id_persona"], errors="coerce").fillna(-1).astype(int)

salida = df[["id_persona", "edad", "sexo", "dosis_previas", "fabricante", "departamento", "periodo", "fallecido"]]
os.makedirs(os.path.dirname(args.output), exist_ok=True)
salida.to_csv(args.output, index=False)

# ---- 8. Codebook: la PRIMERA categoria de cada lista es la de referencia (no genera columna en el one-hot)
codebook = {
    "columnas_csv": list(salida.columns),
    "sexo": ["F", "M"],
    "dosis_previas": ["0", "1", "2", "3+"],
    "fabricante": ["NINGUNA", "PFIZER", "SINOPHARM", "ASTRAZENECA", "OTRO"],
    "departamento": ["LIMA"] + sorted(d for d in salida["departamento"].unique() if d != "LIMA"),
    "periodo": ["2020-S1", "2020-S2", "2021-S1", "2021-S2", "2022+"],
    "edad_min": 0, "edad_max": 110,
    "edad_media": None, "edad_desv": None,   # los completa aumento.py con el conjunto de entrenamiento real
}
with open(args.codebook, "w", encoding="utf-8") as f:
    json.dump(codebook, f, ensure_ascii=False, indent=2)

resumen["filas_final"] = len(salida)
resumen["personas_unicas"] = int(salida["id_persona"].nunique())
resumen["fallecidos"] = int(salida["fallecido"].sum())
resumen["tasa_fallecidos"] = round(float(salida["fallecido"].mean()), 4)
resumen["mortalidad_por_sexo"] = {k: round(float(v), 4) for k, v in salida.groupby("sexo")["fallecido"].mean().items()}
grupos = pd.cut(salida["edad"], [-1, 30, 45, 60, 75, 110], labels=["0-30", "31-45", "46-60", "61-75", "76+"])
resumen["mortalidad_por_edad"] = {str(k): round(float(v), 4) for k, v in salida.groupby(grupos, observed=True)["fallecido"].mean().items()}
resumen["mortalidad_por_departamento"] = {k: round(float(v), 4) for k, v in salida.groupby("departamento")["fallecido"].mean().sort_values(ascending=False).items()}
with open(args.resumen, "w", encoding="utf-8") as f:
    json.dump(resumen, f, ensure_ascii=False, indent=2)
print(f"8. Limpio: {len(salida)} filas, {resumen['personas_unicas']} personas, {resumen['fallecidos']} fallecidos ({resumen['tasa_fallecidos']:.1%})")
print(f"   -> {args.output}, {args.codebook}, {args.resumen}")
