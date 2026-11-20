"""Figuras del analisis de datos para el informe (docs/evidencia/, carpeta local no versionada).

Usa data/raw/TB_HOSP_VAC_FALLECIDOS.csv (para mostrar la fuga de flag_vacuna) y data/hospitalizados_limpio.csv.
Uso: python python/graficos.py
"""
import json
import os

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

RAIZ = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
EVID = os.path.join(RAIZ, "docs", "evidencia")
os.makedirs(EVID, exist_ok=True)
AZUL, ROJO, VERDE, GRIS = "#1f5f8b", "#d1495b", "#2a9d8f", "#8a8f98"
plt.rcParams.update({"font.family": "DejaVu Sans", "font.size": 10, "axes.spines.top": False, "axes.spines.right": False,
                     "axes.grid": True, "grid.alpha": 0.3, "figure.dpi": 150})

limpio = pd.read_csv(os.path.join(RAIZ, "data", "hospitalizados_limpio.csv"))
raw = pd.read_csv(os.path.join(RAIZ, "data", "raw", "TB_HOSP_VAC_FALLECIDOS.csv"), sep=";", encoding="utf-8-sig",
                  usecols=["flag_vacuna", "cdc_fallecido_covid", "fecha_ingreso_hosp"], low_memory=False)
with open(os.path.join(RAIZ, "data", "limpieza_resumen.json"), encoding="utf-8") as f:
    res = json.load(f)


def barras_mortalidad(ax, etiquetas, valores, color, titulo, xlabel):
    ax.bar(etiquetas, [100 * v for v in valores], color=color)
    for i, v in enumerate(valores):
        ax.text(i, 100 * v + 0.8, f"{100*v:.1f} %", ha="center", fontsize=8)
    ax.set_ylabel("Mortalidad (%)"); ax.set_xlabel(xlabel); ax.set_title(titulo); ax.set_ylim(0, max(100 * v for v in valores) * 1.2)


# 1. balance de clases
fig, ax = plt.subplots(figsize=(4.5, 3.6))
conteo = limpio["fallecido"].value_counts().sort_index()
ax.bar(["Sobrevive (0)", "Fallece (1)"], conteo.values, color=[AZUL, ROJO])
for i, v in enumerate(conteo.values):
    ax.text(i, v + 1500, f"{v:,}\n({100*v/len(limpio):.1f} %)".replace(",", " "), ha="center", fontsize=9)
ax.set_ylabel("Hospitalizaciones"); ax.set_title("Variable objetivo: fallecido por COVID-19"); ax.set_ylim(0, conteo.max() * 1.2)
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_balance.png")); plt.close(fig)

# 2. mortalidad por grupo de edad
fig, ax = plt.subplots(figsize=(6, 3.6))
m = res["mortalidad_por_edad"]
barras_mortalidad(ax, list(m.keys()), list(m.values()), ROJO, "Mortalidad por grupo de edad", "Edad (años)")
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_edad.png")); plt.close(fig)

# 3. fuga de informacion: flag_vacuna (original) vs dosis previas al ingreso (recalculadas)
fig, (a1, a2) = plt.subplots(1, 2, figsize=(9, 3.8))
fv = raw.groupby("flag_vacuna")["cdc_fallecido_covid"].mean()
barras_mortalidad(a1, [str(k) for k in fv.index], fv.values, GRIS, "flag_vacuna original (cuenta dosis posteriores al ingreso)", "Dosis registradas a la fecha de corte")
dp = res["mortalidad_por_dosis_previas"]
barras_mortalidad(a2, ["0", "1", "2", "3+"], list(dp.values()), VERDE, "Dosis aplicadas ≥ 14 días antes del ingreso", "Dosis previas al ingreso")
fig.suptitle("Por qué no se usa flag_vacuna: el efecto aparente (62.9 % → 0.4 %) es una fuga de información", fontsize=10)
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_fuga_vacuna.png")); plt.close(fig)

# 4. periodo (ola): volumen y mortalidad
fig, ax1 = plt.subplots(figsize=(7, 3.8))
per = res["periodo"]; mp = res["mortalidad_por_periodo"]
ax1.bar(list(per.keys()), list(per.values()), color=AZUL, alpha=0.85, label="Hospitalizaciones")
ax1.set_ylabel("Hospitalizaciones"); ax1.set_xlabel("Periodo de ingreso")
ax2 = ax1.twinx(); ax2.grid(False)
ax2.plot(list(mp.keys()), [100 * v for v in mp.values()], "o-", color=ROJO, label="Mortalidad (%)")
ax2.set_ylabel("Mortalidad (%)"); ax2.set_ylim(0, 60)
h1, l1 = ax1.get_legend_handles_labels(); h2, l2 = ax2.get_legend_handles_labels(); ax1.legend(h1 + h2, l1 + l2, loc="upper right")
ax1.set_title("Olas de la pandemia: volumen y mortalidad por periodo de ingreso")
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_periodo.png")); plt.close(fig)

# 5. departamento (mortalidad y volumen)
fig, ax = plt.subplots(figsize=(9, 4))
md = pd.Series(res["mortalidad_por_departamento"])
vol = limpio["departamento"].value_counts()
orden = md.sort_values(ascending=False).index
ax.bar(orden, [100 * md[d] for d in orden], color=[ROJO if vol[d] >= 2000 else GRIS for d in orden])
ax.axhline(100 * res["tasa_fallecidos"], color=AZUL, ls="--", lw=1, label=f"Promedio nacional {100*res['tasa_fallecidos']:.1f} %")
ax.set_ylabel("Mortalidad (%)"); ax.set_title("Mortalidad por departamento de domicilio (gris: menos de 2 000 hospitalizaciones)")
ax.tick_params(axis="x", rotation=75, labelsize=8); ax.legend()
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_departamento.png")); plt.close(fig)

# 6. sexo y fabricante
fig, (a1, a2) = plt.subplots(1, 2, figsize=(9, 3.6))
ms = res["mortalidad_por_sexo"]
barras_mortalidad(a1, ["Femenino", "Masculino"], [ms["F"], ms["M"]], AZUL, "Mortalidad por sexo", "")
mf = limpio.groupby("fabricante")["fallecido"].mean().reindex(["NINGUNA", "PFIZER", "SINOPHARM", "ASTRAZENECA", "OTRO"]).dropna()
barras_mortalidad(a2, list(mf.index), list(mf.values), VERDE, "Mortalidad por fabricante de la primera dosis previa", "")
a2.tick_params(axis="x", labelsize=8)
fig.tight_layout(); fig.savefig(os.path.join(EVID, "datos_sexo_fabricante.png")); plt.close(fig)

print("Figuras en", EVID, ":", sorted(f for f in os.listdir(EVID) if f.startswith("datos_")))
