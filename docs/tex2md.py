#!/usr/bin/env python3
"""Produit GUIDE.fr.md à partir de docs/guide-fr.tex.

Le LaTeX est la source du guide français ; ce script en tire la version
que GitHub sait afficher. Il ne fait que nettoyer ce que pandoc laisse
derrière lui : les renvois en HTML et le bloc de titre.

Usage : python3 docs/tex2md.py  (depuis la racine du dépôt)
"""
import re
import subprocess
import sys

TEX = "docs/guide-fr.tex"
MD = "GUIDE.fr.md"

ENTETE = """<!-- Fichier produit par « make guide-fr-md » depuis docs/guide-fr.tex.
     Les modifications faites ici seront perdues à la prochaine génération. -->

# Guide de la bibliothèque *timeseries*

**Traitement des séries temporelles à trous** — Frédéric Flament

*Ce guide est écrit en LaTeX ; la version composée, avec sa table des
matières et sa bibliographie, se construit par `make guide-fr`. La
version anglaise est dans [GUIDE.md](GUIDE.md).*

"""

def main() -> int:
    texte = subprocess.run(
        ["pandoc", TEX, "-f", "latex", "-t", "gfm", "--citeproc",
         "--bibliography=docs/references.bib", "--wrap=none",
         "--shift-heading-level-by=1"],
        capture_output=True, text=True, check=True).stdout

    # Les renvois de LaTeX sortent en HTML ; seul le numéro nous intéresse.
    texte = re.sub(r'<a href="#[^"]*" data-reference-type="[^"]*" '
                   r'data-reference="[^"]*">([^<]*)</a>', r"\1", texte)

    # Le bloc de titre centré est remplacé par un titre Markdown.
    fin_titre = texte.find("## Introduction")
    if fin_titre == -1:
        print("le chapitre Introduction est introuvable", file=sys.stderr)
        return 1
    # L'abstract est le seul bloc de citation avant l'introduction.
    resume = re.search(r"\n((?:> ?.*\n)+)", texte[:fin_titre])
    corps = ENTETE
    if resume:
        # pandoc en fait une citation ; ici il se présente seul, sans intitulé.
        texte_resume = re.sub(r"^> ?", "", resume.group(1).strip(), flags=re.M)
        corps += texte_resume + "\n\n"
    corps += texte[fin_titre:]

    open(MD, "w").write(corps)
    print(f"→ {MD}")
    return 0

if __name__ == "__main__":
    sys.exit(main())
