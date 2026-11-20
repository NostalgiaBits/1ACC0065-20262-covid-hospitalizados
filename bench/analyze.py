"""Analiza bench/results.csv: media recortada (10 %), speedup y eficiencia de la carga y del entrenamiento por separado.

Salida: bench/tabla.md, bench/resumen.json y figuras en docs/evidencia/ (carpeta local, no versionada).
Uso: python bench/analyze.py
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

# paleta (misma que en los entregables anteriores)
C_CARGA, C_ENTREN, C_IDEAL, C_GRIS = "#1f5f8b", "#d1495b", "#8a8f98", "#c9ced6"
plt.rcParams.update({"font.family": "DejaVu Sans", "font.size": 10, "axes.spines.top": False, "axes.spines.right": False,
                     "axes.grid": True, "grid.alpha": 0.3, "figure.dpi": 150})


def media_recortada(s, frac=0.10):
    v = np.sort(np.asarray(s, dtype=float))
    k = int(len(v) * frac)
    return float(v[k:len(v) - k].mean()) if len(v) - 2 * k > 0 else float(v.mean())


df = pd.read_csv(os.path.join(RAIZ, "bench", "results.csv"))
g = df.groupby("workers")
tabla = pd.DataFrame({
    "corridas": g.size(),
    "carga_s": g["carga_ms"].apply(media_recortada) / 1000,
    "entren_s": g["entren_ms"].apply(media_recortada) / 1000,
    "entren_desv_s": g["entren_ms"].std() / 1000,
    "perdida": g["perdida"].first(),
    "auc": g["auc"].first(),
    "cpu_s": g["cpu_s"].apply(media_recortada),
    "mem_mb": g["mem_max_mb"].apply(media_recortada),
}).sort_index()
base_c, base_e = tabla.loc[1, "carga_s"], tabla.loc[1, "entren_s"]
tabla["speedup_carga"] = base_c / tabla["carga_s"]
tabla["speedup_entren"] = base_e / tabla["entren_s"]
tabla["eficiencia_entren"] = tabla["speedup_entren"] / tabla.index
tabla["total_s"] = tabla["carga_s"] + tabla["entren_s"]
tabla["speedup_total"] = tabla.loc[1, "total_s"] / tabla["total_s"]
mejor = int(tabla["speedup_entren"].idxmax())

# ---- tabla Markdown
lineas = ["| Workers | Carga (s) | Speedup carga | Entrenamiento (s) | Speedup entren. | Eficiencia | Total (s) | Speedup total | Pérdida final | AUC | CPU (s) | Memoria máx. (MB) |",
          "|---|---|---|---|---|---|---|---|---|---|---|---|"]
for w, r in tabla.iterrows():
    neg = "**" if w == mejor else ""
    lineas.append(f"| {neg}{w}{neg} | {r.carga_s:.3f} | {r.speedup_carga:.2f}× | {r.entren_s:.3f} | {neg}{r.speedup_entren:.2f}×{neg} | "
                  f"{100*r.eficiencia_entren:.0f} % | {r.total_s:.3f} | {r.speedup_total:.2f}× | {r.perdida:.6f} | {r.auc:.4f} | {r.cpu_s:.1f} | {r.mem_mb:.0f} |")
with open(os.path.join(RAIZ, "bench", "tabla.md"), "w", encoding="utf-8") as f:
    f.write("\n".join(lineas) + "\n")
print("\n".join(lineas))

resumen = {"mejor_workers": mejor, "epocas": int(df["epocas"].iloc[0]), "repeticiones_por_config": int(tabla["corridas"].min()),
           "perdida_identica": bool(df["perdida"].nunique() == 1), "auc_identica": bool(df["auc"].nunique() == 1),
           "por_workers": {str(w): {k: (round(float(v), 4)) for k, v in r.items()} for w, r in tabla.iterrows()}}
with open(os.path.join(RAIZ, "bench", "resumen.json"), "w", encoding="utf-8") as f:
    json.dump(resumen, f, ensure_ascii=False, indent=2)

W = tabla.index.values

# ---- figura 1: tiempos
fig, ax = plt.subplots(figsize=(7, 4))
ax.plot(W, tabla["entren_s"], "o-", color=C_ENTREN, label="Entrenamiento (Parte B)")
ax.plot(W, tabla["carga_s"], "s-", color=C_CARGA, label="Carga (Parte A)")
ax.set_xlabel("Goroutines (workers)"); ax.set_ylabel("Tiempo (s), media recortada 10 %"); ax.set_xticks(W)
ax.set_title(f"Tiempo por etapa: {resumen['epocas']} épocas sobre 1 050 000 registros"); ax.legend()
fig.tight_layout(); fig.savefig(os.path.join(EVID, "bench_tiempos.png")); plt.close(fig)

# ---- figura 2: speedup
fig, ax = plt.subplots(figsize=(7, 4))
ax.plot(W, W, "--", color=C_IDEAL, label="Ideal (lineal)")
ax.plot(W, tabla["speedup_entren"], "o-", color=C_ENTREN, label="Entrenamiento")
ax.plot(W, tabla["speedup_carga"], "s-", color=C_CARGA, label="Carga")
ax.plot(W, tabla["speedup_total"], "^-", color="#2a9d8f", label="Total")
ax.annotate(f"máx. {tabla.loc[mejor, 'speedup_entren']:.2f}× (W = {mejor})", (mejor, tabla.loc[mejor, "speedup_entren"]),
            textcoords="offset points", xytext=(-80, 10), arrowprops={"arrowstyle": "->", "color": C_GRIS})
ax.set_xlabel("Goroutines (workers)"); ax.set_ylabel("Speedup = T(1) / T(W)"); ax.set_xticks(W); ax.legend()
ax.set_title("Speedup frente a la ejecución con un worker")
fig.tight_layout(); fig.savefig(os.path.join(EVID, "bench_speedup.png")); plt.close(fig)

# ---- figura 3: eficiencia
fig, ax = plt.subplots(figsize=(7, 3.6))
ax.bar([str(w) for w in W], 100 * tabla["eficiencia_entren"], color=C_ENTREN)
for i, v in enumerate(tabla["eficiencia_entren"]):
    ax.text(i, 100 * v + 1, f"{100*v:.0f} %", ha="center", fontsize=8)
ax.set_xlabel("Goroutines (workers)"); ax.set_ylabel("Eficiencia = speedup / W (%)"); ax.set_ylim(0, 110)
ax.set_title("Eficiencia del entrenamiento paralelo")
fig.tight_layout(); fig.savefig(os.path.join(EVID, "bench_eficiencia.png")); plt.close(fig)

# ---- figura 4: CPU y memoria
fig, ax1 = plt.subplots(figsize=(7, 3.6))
ax1.bar([str(w) for w in W], tabla["cpu_s"], color=C_CARGA, alpha=0.85, label="CPU total del proceso (s)")
ax1.set_xlabel("Goroutines (workers)"); ax1.set_ylabel("Tiempo de CPU (s)")
ax2 = ax1.twinx(); ax2.grid(False)
ax2.plot([str(w) for w in W], tabla["mem_mb"], "o-", color=C_ENTREN, label="Memoria máxima (MB)")
ax2.set_ylabel("Memoria (MB)"); ax2.set_ylim(0, tabla["mem_mb"].max() * 1.3)
h1, l1 = ax1.get_legend_handles_labels(); h2, l2 = ax2.get_legend_handles_labels(); ax1.legend(h1 + h2, l1 + l2, loc="upper left")
ax1.set_title("Recursos por configuración")
fig.tight_layout(); fig.savefig(os.path.join(EVID, "bench_recursos.png")); plt.close(fig)

print(f"\nMejor: W = {mejor} ({tabla.loc[mejor, 'speedup_entren']:.2f}x en entrenamiento). Pérdida idéntica en todas las corridas: "
      f"{resumen['perdida_identica']}. Figuras en {EVID}")
