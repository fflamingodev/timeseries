# Construction des guides en PDF.
#
# Le Markdown reste la source : GitHub et Typora le rendent tel quel, et
# git en suit les modifications ligne à ligne. LaTeX n'est qu'un format
# de sortie, produit par pandoc avec le préambule de docs/.
#
#   make guides     les deux PDF
#   make guide-fr   le guide français
#   make guide-en   le guide anglais
#   make clean      efface les PDF

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

.PHONY: guides guide-fr guide-en clean

guides: guide-fr guide-en

guide-fr: GUIDE.fr.pdf
guide-en: GUIDE.pdf

GUIDE.fr.pdf: GUIDE.fr.md docs/preamble.tex docs/guide.lua
	$(PANDOC) $< -o $@ $(FLAGS) -V lang=fr
	@echo "→ $@"

GUIDE.pdf: GUIDE.md docs/preamble.tex docs/guide.lua
	$(PANDOC) $< -o $@ $(FLAGS) -V lang=en
	@echo "→ $@"

clean:
	rm -f GUIDE.pdf GUIDE.fr.pdf
