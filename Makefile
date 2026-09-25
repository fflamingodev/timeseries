# Construction des guides.
#
# Deux chaînes, parce que les deux guides n'ont pas la même source.
#
#   Français : docs/guide-fr.tex est la source. latexmk en tire le PDF,
#              et docs/tex2md.py en tire GUIDE.fr.md, que GitHub affiche.
#              Ne pas modifier GUIDE.fr.md : il est écrasé.
#
#   Anglais  : GUIDE.md est la source. pandoc en tire GUIDE.pdf, avec le
#              préambule docs/preamble.tex et le filtre docs/guide.lua.
#
#   make guides       tout
#   make guide-fr     le PDF français et son Markdown
#   make guide-fr-md  le Markdown français seul
#   make guide-en     le PDF anglais
#   make clean        efface les PDF et les fichiers de travail de LaTeX
#
# Il faut pandoc, une distribution TeX (xelatex, biber, latexmk), python3,
# et la police Menlo pour les caractères semi-graphiques.

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
