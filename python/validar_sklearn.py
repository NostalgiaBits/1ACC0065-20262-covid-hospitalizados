"""Referencia con scikit-learn: la misma regresion logistica (misma codificacion que Go) para validar la implementacion en Go.

Codificacion identica a comun/datos.go: edad estandarizada con media/desv del codebook; one-hot con la primera categoria
de cada lista como referencia (sin columna). Sin regularizacion (penalty=None), igual que el descenso de gradiente en Go.

Salida: data/sklearn_referencia.json (metricas en test y coeficientes por nombre de columna)
Uso: python python/validar_sklearn.py
"""
import argparse
import json
import time

import numpy as np
import pandas as pd
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import roc_auc_score, f1_score, precision_score, recall_score, accuracy_score, confusion_matrix

ap = argparse.ArgumentParser()
ap.add_argument("--train", default="data/train_1M.csv")
ap.add_argument("--test", default="data/test.csv")
ap.add_argument("--codebook", default="data/codebook.json")
ap.add_argument("--salida", default="data/sklearn_referencia.json")
args = ap.parse_args()

with open(args.codebook, encoding="utf-8") as f:
    cb = json.load(f)


def codificar(df):
    """Devuelve (X, nombres) con el mismo orden de columnas que comun/datos.go."""
    cols, nombres = [], []
    cols.append(((df["edad"] - cb["edad_media"]) / cb["edad_desv"]).values); nombres.append("edad_z")
    for nombre, lista, col in (("sexo", cb["sexo"], df["sexo"].astype(str)),
                               ("dosis", cb["dosis_previas"], df["dosis_previas"].astype(str).replace("3", "3+")),
                               ("fabricante", cb["fabricante"], df["fabricante"].astype(str)),
                               ("dep", cb["departamento"], df["departamento"].astype(str)),
                               ("periodo", cb["periodo"], df["periodo"].astype(str))):
        for cat in lista[1:]:  # la primera categoria es la referencia
            cols.append((col == cat).astype(float).values); nombres.append(f"{nombre}={cat}")
    return np.column_stack(cols), nombres


train = pd.read_csv(args.train)
test = pd.read_csv(args.test)
Xtr, nombres = codificar(train)
Xte, _ = codificar(test)
ytr, yte = train["fallecido"].values, test["fallecido"].values
print(f"train {Xtr.shape}, test {Xte.shape}, columnas: {len(nombres)}")

resultado = {"columnas": nombres, "modelos": {}}
for etiqueta, mascara in (("train_1M", np.ones(len(train), dtype=bool)), ("train_real", train["id_persona"].values > 0)):
    t0 = time.time()
    clf = LogisticRegression(penalty=None, max_iter=3000, tol=1e-6)
    clf.fit(Xtr[mascara], ytr[mascara])
    p = clf.predict_proba(Xte)[:, 1]
    pred = (p >= 0.5).astype(int)
    cm = confusion_matrix(yte, pred)
    m = {"filas_entrenamiento": int(mascara.sum()), "segundos": round(time.time() - t0, 1),
         "auc": round(float(roc_auc_score(yte, p)), 4), "accuracy": round(float(accuracy_score(yte, pred)), 4),
         "precision": round(float(precision_score(yte, pred)), 4), "recall": round(float(recall_score(yte, pred)), 4),
         "f1": round(float(f1_score(yte, pred)), 4),
         "matriz_confusion": {"vn": int(cm[0, 0]), "fp": int(cm[0, 1]), "fn": int(cm[1, 0]), "vp": int(cm[1, 1])},
         "sesgo": round(float(clf.intercept_[0]), 4),
         "coeficientes": {n: round(float(c), 4) for n, c in zip(nombres, clf.coef_[0])}}
    resultado["modelos"][etiqueta] = m
    print(f"{etiqueta}: AUC {m['auc']}, acc {m['accuracy']}, prec {m['precision']}, rec {m['recall']}, F1 {m['f1']} ({m['segundos']} s)")

with open(args.salida, "w", encoding="utf-8") as f:
    json.dump(resultado, f, ensure_ascii=False, indent=2)
print("->", args.salida)
