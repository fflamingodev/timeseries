# Construction du guide.
#
# docs/guide-fr.tex est la source, et la seule. latexmk en tire le PDF,
# et docs/tex2md.py en tire README.md, la page d'accueil du dépôt et
# celle que pkg.go.dev affiche. Ne pas modifier README.md : il est
# écrasé à chaque génération.
#
# L'ordre compte. Markdown ne sait ni numéroter, ni renvoyer, ni dresser
# une table des matières : tex2md.py reprend tout cela de
# docs/guide-fr.toc et docs/guide-fr.aux, que LaTeX écrit en composant.
# Le Markdown dépend donc du PDF, et non du .tex. Si les deux ne
# s'accordent pas sur le nombre de titres, tex2md.py s'arrête plutôt que
# de décaler la numérotation.
#
# Les citations du Markdown suivent docs/ieee.csl, un style numérique,
# pour porter les mêmes numéros que la bibliographie du PDF, que
# biblatex numérote.
#
# Les annexes sont particulières : docs/annexes.tex porte la prose, mais
# le code qu'elle présente est extrait des paquets par docs/gendoc.py,
# dans docs/generated/. Ni les signatures ni les exemples ne sont saisis
# à la main, donc ni l'un ni l'autre ne peut mentir sur l'état du code.
#
#   make              le PDF et le README
#   make guide-fr     la même chose, nommément
#   make guide-fr-md  le README seul
#   make gendoc       les annexes extraites du code
#   make clean        efface le PDF, les annexes extraites et les
#                     fichiers de travail de LaTeX
#
# Il faut pandoc, une distribution TeX (xelatex, biber, latexmk), python3,
# et la police Menlo pour les caractères semi-graphiques.

# xelatex vit dans /Library/TeX/texbin, absent du PATH d'un shell non interactif.
export PATH := /Library/TeX/texbin:$(PATH)

# Les sources dont les annexes sont tirées : le code des deux paquets,
# les exemples compris.
SOURCES := $(wildcard *.go) $(wildcard ../notavalue/*.go)

# Un témoin plutôt que la liste des fichiers produits, qui change dès
# qu'un exemple est ajouté.
TEMOIN := docs/generated/.a-jour

.PHONY: guide-fr guide-fr-md gendoc clean

guide-fr: docs/guide-fr.pdf README.md
gendoc: $(TEMOIN)

# Les annexes sortent de « go doc » et de example_test.go.
$(TEMOIN): docs/gendoc.py $(SOURCES)
	python3 docs/gendoc.py
	@touch $@

# Le guide français est écrit en LaTeX : biblatex numérote la
# bibliographie, \ref les renvois, et la date se met à jour seule.
docs/guide-fr.pdf: docs/guide-fr.tex docs/annexes.tex docs/references.bib $(TEMOIN)
	cd docs && latexmk -xelatex -interaction=nonstopmode guide-fr.tex
	@echo "→ $@"

# README.md est produit, pas écrit : la source est le LaTeX.
guide-fr-md: README.md

# Dépend du PDF, pas du .tex : le script a besoin du .toc et du .aux que
# latexmk vient d'écrire.
README.md: docs/guide-fr.pdf docs/tex2md.py docs/ieee.csl go.mod
	python3 docs/tex2md.py

clean:
	rm -rf docs/generated
	cd docs && latexmk -C guide-fr.tex
