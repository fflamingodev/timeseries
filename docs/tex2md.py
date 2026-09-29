#!/usr/bin/env python3
"""Produit README.md à partir de docs/guide-fr.tex.

Le LaTeX est la source du guide français ; ce script en tire la version
que GitHub sait afficher. Markdown ne sait ni numéroter, ni renvoyer, ni
dresser une table des matières : tout cela est repris ici des fichiers
que LaTeX vient d'écrire, de sorte que les deux versions portent les
mêmes numéros.

Ce qu'il fait, dans l'ordre :

  1. déshabille les entêtes de table héritées de pandoc, sans quoi onze
     tables sur seize sortent en HTML (voir sans_minipage) ;
  2. appelle pandoc, avec un style de citation numérique pour que les
     renvois bibliographiques soient les mêmes que dans le PDF ;
  3. numérote les titres d'après docs/guide-fr.toc ;
  4. dresse la table des matières, avec des liens vers les ancres que
     GitHub fabrique à partir des titres ;
  5. numérote les intitulés de table d'après docs/guide-fr.aux ;
  6. remplace les \\<div\\> de pandoc par ce qu'il faut de plus discret :
     une ancre d'une ligne pour les tables, une liste numérotée pour la
     bibliographie.

Il lui faut donc le .toc et le .aux, c'est-à-dire un PDF déjà construit.
Le Makefile s'en charge : README.md dépend de docs/guide-fr.pdf.

Usage : python3 docs/tex2md.py  (depuis la racine du dépôt)
"""
import os
import re
import subprocess
import sys

TEX = "docs/guide-fr.tex"
AUX = "docs/guide-fr.aux"
TOC = "docs/guide-fr.toc"
CSL = "ieee.csl"          # relatif à docs/, où pandoc est lancé
MD = "README.md"

# Le décalage de pandoc : un \chapter devient ##, une \section ###, une
# \subsection ####.
DIESES = {"chapter": "##", "section": "###", "subsection": "####"}

ENTETE = """<!-- Fichier produit par « make guide-fr-md » depuis docs/guide-fr.tex.
     Les modifications faites ici seront perdues à la prochaine génération. -->

# {titre}

**{soustitre}** — {auteur}

```go
import "{module}"
```

*Ce document est le guide de la bibliothèque. La référence des
interfaces, avec les exemples exécutables, est sur
[pkg.go.dev](https://pkg.go.dev/{module}) ; la version composée en PDF,
avec sa pagination et sa bibliographie, se construit par
`make guide-fr`.*

"""

MINIPAGE = re.compile(
    r"\\begin\{minipage\}\[b\]\{\\linewidth\}\\raggedright\s*\n?"
    r"|\s*\\end\{minipage\}")

RENVOI = re.compile(r'<a href="#(?P<cible>[^"]*)" data-reference-type="[^"]*" '
                    r'data-reference="[^"]*">(?P<texte>[^<]*)</a>')

TITRE = re.compile(r"^(#{2,4}) (.+)$", re.M)

# \contentsline {section}{\numberline {2.1}Introduction}{6}{...}
LIGNE_TOC = re.compile(
    r"\\contentsline \{(chapter|section|subsection)\}"
    r"\{\\numberline \{([^}]*)\}(.*?)\}\{\d+\}")

# Ce que le générateur d'ancres de GitHub retire d'un titre. Tout ce qui
# n'est ni lettre, ni chiffre, ni espace, ni tiret, ni souligné : les
# accents restent, les deux-points, guillemets et points disparaissent.
PONCTUATION = re.compile(r"[^\w\- ]", re.UNICODE)


def metadonnee(source: str, nom: str) -> str:
    """Lit \\newcommand{\\nom}{...} dans le LaTeX.

    Le titre, le sous-titre et l'auteur ne sont écrits qu'à un endroit,
    le préambule du document ; les recopier ici les ferait diverger au
    premier remaniement.
    """
    m = re.search(r"\\newcommand\{\\" + nom + r"\}\{(.*?)\}\s*\n(?=\\|%|\n)",
                  source, re.S)
    if not m:
        raise SystemExit(f"métadonnée {nom} introuvable dans {TEX}")
    texte = m.group(1)
    texte = re.sub(r"\\textit\{([^}]*)\}", r"*\1*", texte)
    texte = re.sub(r"\\texttt\{([^}]*)\}", r"`\1`", texte)
    return " ".join(texte.split())


def chemin_du_module(chemin: str = "go.mod") -> str:
    """Lit le chemin du module dans go.mod.

    Le README annonce l'import et renvoie à pkg.go.dev : deux endroits
    de plus où écrire un chemin qui changerait sans prévenir.
    """
    m = re.search(r"^module\s+(\S+)", open(chemin).read(), re.M)
    if not m:
        raise SystemExit(f"chemin du module introuvable dans {chemin}")
    return m.group(1)


def sans_minipage(source: str) -> str:
    """Déshabille les cellules d'entête des tables héritées de pandoc.

    Les \\begin{minipage} qui enveloppent les entêtes datent de l'époque
    où le guide était écrit en Markdown : pandoc les avait produits,
    LaTeX s'en accommode. Mais au retour, une cellule qui contient un
    bloc ne rentre pas dans une table Markdown, et pandoc se rabat sur
    du HTML. Onze des seize tables sortaient ainsi en \\<table\\>.

    Le déshabillage se fait sur une copie, dans le tampon passé à
    pandoc. La source LaTeX n'est pas touchée, donc le PDF ne bouge pas.
    """
    return MINIPAGE.sub("", source)


def ardoise(titre: str) -> str:
    """Rend l'ancre que GitHub fabrique pour un titre.

    L'algorithme est celui de github-slugger : passage en minuscules,
    suppression de la ponctuation, espaces changés en tirets. Les
    tirets consécutifs ne sont pas fondus, d'où « a--b » quand deux
    espaces se suivaient.

    Faute de pouvoir l'éprouver contre GitHub depuis ici, c'est
    l'endroit à vérifier si un lien de la table des matières tombe à
    côté.
    """
    texte = titre.replace("`", "").replace("*", "")
    return PONCTUATION.sub("", texte.lower()).replace(" ", "-")


def entrees_du_toc(chemin: str) -> list[tuple[str, str, str]]:
    """Lit (niveau, numéro, titre) dans le .toc, dans l'ordre du document.

    Seules les entrées numérotées sont retenues. La bibliographie n'en a
    pas, et pandoc ne lui donne pas de titre non plus : les deux listes
    se correspondent donc terme à terme.
    """
    try:
        source = open(chemin).read()
    except FileNotFoundError:
        raise SystemExit(f"{chemin} absent : construire le PDF d'abord "
                         "(« make guide-fr »)")
    return [(niveau, numero, titre)
            for niveau, numero, titre in LIGNE_TOC.findall(source)]


def numerote_les_titres(texte: str,
                        entrees: list[tuple[str, str, str]]) -> str:
    """Préfixe chaque titre du Markdown par son numéro de LaTeX.

    L'appariement se fait par rang et par niveau : les deux listes
    décrivent le même document dans le même ordre. Un désaccord sur le
    nombre ou sur le niveau signifie que l'une des deux est périmée, et
    numéroter à l'aveugle décalerait tout le document — on s'arrête.
    """
    titres = TITRE.findall(texte)
    if len(titres) != len(entrees):
        raise SystemExit(
            f"{len(titres)} titres dans le Markdown contre {len(entrees)} "
            f"dans {TOC} : le PDF et le Markdown ne décrivent pas la même "
            "version. Relancer « make guide-fr ».")

    for rang, ((dieses, _), (niveau, _, _)) in enumerate(zip(titres, entrees)):
        if DIESES[niveau] != dieses:
            raise SystemExit(
                f"titre n° {rang + 1} : « {dieses} » dans le Markdown, "
                f"{niveau} dans {TOC}")

    numeros = iter(entrees)

    def remplace(m: re.Match) -> str:
        dieses, titre = m.group(1), m.group(2)
        _, numero, _ = next(numeros)
        return f"{dieses} {numero} {titre}"

    return TITRE.sub(remplace, texte)


def table_des_matieres(texte: str) -> str:
    """Dresse la table des matières à partir des titres déjà numérotés."""
    lignes = ["## Table des matières\n"]
    for dieses, titre in TITRE.findall(texte):
        if titre.startswith("Table des matières"):
            continue
        creux = "  " * (len(dieses) - 2)
        lignes.append(f"{creux}- [{titre}](#{ardoise(titre)})")
    return "\n".join(lignes) + "\n"


def numeros_de_table(chemin: str) -> dict[str, str]:
    """Lit les numéros de table dans le .aux de LaTeX.

    Sans cela, le texte renvoie à « la table 2.4 » et aucune table
    n'annonce son numéro.
    """
    try:
        aux = open(chemin).read()
    except FileNotFoundError:
        raise SystemExit(f"{chemin} absent : construire le PDF d'abord "
                         "(« make guide-fr »)")
    return dict(re.findall(r"\\newlabel\{(tab:[^}]*)\}\{\{([^}]*)\}", aux))


def renvoi_markdown(m: re.Match) -> str:
    """Rend un renvoi de LaTeX.

    pandoc écrit une ancre HTML vers l'étiquette. Les tables sont les
    seules à recevoir une ancre de destination : le lien y fonctionne.
    Les chapitres et sections n'en reçoivent aucune, et un lien mort
    vaut moins qu'un numéro nu.
    """
    cible, texte = m.group("cible"), m.group("texte")
    if cible.startswith("tab:"):
        return f"[{texte}](#{cible})"
    return texte


def habille_les_tables(texte: str, numeros: dict[str, str]) -> str:
    """Numérote l'intitulé de chaque table et allège son enveloppe.

    pandoc enferme la table et son intitulé dans un \\<div\\> qui porte
    l'étiquette. Le \\<div\\> tient trois lignes ; une ancre vide en tient
    une, et suffit à ce que les renvois arrivent au bon endroit.
    """
    def remplace(m: re.Match) -> str:
        etiquette, corps = m.group(1), m.group(2).strip("\n")
        lignes = corps.split("\n")
        numero = numeros.get(etiquette)
        if numero:
            lignes[-1] = f"**Table {numero}** — {lignes[-1]}"
        return f'<a id="{etiquette}"></a>\n\n' + "\n".join(lignes)

    return re.sub(r'<div id="(tab:[^"]*)">\n(.*?)\n</div>',
                  remplace, texte, flags=re.S)


def bibliographie(texte: str) -> str:
    """Transforme la bibliographie de citeproc en liste numérotée.

    citeproc rend un \\<div\\> par référence, chacun contenant deux
    \\<span\\> --- le numéro dans la marge, le texte à côté. Dix-sept
    \\<div\\> pour une liste : autant l'écrire en Markdown, avec les
    numéros du style, qui sont ceux cités dans le texte.
    """
    # La bibliographie ferme le document : on prend tout jusqu'au bout
    # plutôt que de compter les \\</div\\> imbriqués.
    bloc = re.search(r'<div id="refs"[^>]*>\n(.*)$', texte, re.S)
    if not bloc:
        return texte

    entrees = re.findall(
        r'<span class="csl-left-margin">\[(\d+)\] ?</span>'
        r'<span class="csl-right-inline">(.*?)</span>',
        bloc.group(1), re.S)
    if not entrees:
        return texte

    liste = "## Bibliographie\n\n" + "\n".join(
        f"{numero}. {corps.strip()}" for numero, corps in entrees) + "\n"
    return texte[:bloc.start()] + liste + texte[bloc.end():]


def main() -> int:
    if not os.path.isdir("docs"):
        print("à lancer depuis la racine du dépôt", file=sys.stderr)
        return 1

    source = open(TEX).read()
    entete = ENTETE.format(titre=metadonnee(source, "montitre"),
                           soustitre=metadonnee(source, "monsoustitre"),
                           auteur=metadonnee(source, "monauteur"),
                           module=chemin_du_module())

    texte = subprocess.run(
        ["pandoc", "-f", "latex", "-t", "gfm", "--citeproc",
         "--bibliography=references.bib", f"--csl={CSL}", "--wrap=none",
         "--shift-heading-level-by=1"],
        input=sans_minipage(source),
        # Depuis docs/, et non depuis la racine : pandoc résout les
        # \input des annexes et le style de citation relativement à son
        # répertoire courant, que --resource-path ne change pas.
        cwd="docs",
        capture_output=True, text=True, check=True).stdout

    texte = RENVOI.sub(renvoi_markdown, texte)
    # Les crochets du style numérique sortent échappés ; ils se lisent
    # mieux nus, et GitHub ne les prend pas pour un lien.
    texte = re.sub(r"\\\[(\d+|Online)\\\]", r"[\1]", texte)
    texte = habille_les_tables(texte, numeros_de_table(AUX))

    # Le bloc de titre de LaTeX est remplacé par un titre Markdown.
    fin_titre = texte.find("## Introduction")
    if fin_titre == -1:
        print("le chapitre Introduction est introuvable", file=sys.stderr)
        return 1
    # L'abstract est le seul bloc de citation avant l'introduction.
    resume = re.search(r"\n((?:> ?.*\n)+)", texte[:fin_titre])

    # La numérotation d'abord : elle compte les titres et les compare à
    # ceux du .toc. La bibliographie, qui en ajoute un que LaTeX ne
    # numérote pas, vient donc après.
    corps = numerote_les_titres(texte[fin_titre:], entrees_du_toc(TOC))
    corps = bibliographie(corps)

    sortie = entete
    if resume:
        # pandoc en fait une citation ; ici il se présente seul, sans intitulé.
        sortie += re.sub(r"^> ?", "", resume.group(1).strip(), flags=re.M)
        sortie += "\n\n"
    sortie += table_des_matieres(corps) + "\n" + corps

    open(MD, "w").write(sortie)
    print(f"→ {MD}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
