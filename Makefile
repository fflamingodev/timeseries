# Construction des guides en PDF.
#
# Le Markdown reste la source : GitHub et Typora le rendent tel quel, et
# git en suit les modifications ligne à ligne. LaTeX n'est qu'un format
# de sortie, produit par pandoc avec le préambule de docs/.
#
#   make guides     les deux PDF
#   make guide-fr-md  régénère GUIDE.fr.md depuis le LaTeX
#   make guide-fr   le guide français, source LaTeX (docs/guide-fr.tex)
#   make guide-en   le guide anglais, converti depuis GUIDE.md
#   make clean      efface les PDF et les fichiers intermédiaires de LaTeX

PANDOC  := pandoc
FLAGS   := --pdf-engine=xelatex \
           --shift-heading-level-by=-1 \
           --toc --toc-depth=2 --number-sections \
           --lua-filter=docs/guide.lua \
           --include-in-header=docs/preamble.tex \
           -V geometry:margin=2.5cm \
           -V monofont="Menlo" \
           -V fontsize=11pt \
           -V colorlinks=true -V linkcolor=black -V urlcolor=Maroon -V toccolor=black \
           -M author="Frédéric Flament"

# xelatex vit dans /Library/TeX/texbin, absent du PATH d'un shell non interactif.
export PATH := /Library/TeX/texbin:$(PATH)

.PHONY: guides guide-fr guide-fr-md guide-en clean

guides: guide-fr guide-en

guide-fr: docs/guide-fr.pdf GUIDE.fr.md
guide-en: GUIDE.pdf

# Le guide français est écrit en LaTeX : biblatex numérote la
# bibliographie, \ref les renvois, et la date se met à jour seule.
docs/guide-fr.pdf: docs/guide-fr.tex docs/references.bib
	cd docs && latexmk -xelatex -interaction=nonstopmode guide-fr.tex
	@echo "→ $@"

# GUIDE.fr.md est produit, pas écrit : la source est le LaTeX.
guide-fr-md: GUIDE.fr.md

GUIDE.fr.md: docs/guide-fr.tex docs/references.bib docs/tex2md.py
	python3 docs/tex2md.py

GUIDE.pdf: GUIDE.md docs/preamble.tex docs/guide.lua
	$(PANDOC) $< -o $@ $(FLAGS) -V lang=en
	@echo "→ $@"

clean:
	rm -f GUIDE.pdf
	cd docs && latexmk -C guide-fr.tex
