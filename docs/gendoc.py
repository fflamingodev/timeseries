#!/usr/bin/env python3
"""Produit les annexes du guide à partir du code, pas de la mémoire.

Deux annexes sont générées, pour les deux paquets :

  * la liste des signatures, extraite de « go doc -all ». Elle sort du
    code compilé : elle ne peut pas décrire une fonction qui n'existe
    plus, ni oublier celle qu'on vient d'ajouter.

  * les exemples d'utilisation, extraits des fichiers example_test.go.
    Ce sont de vraies fonctions Example : « go test » les exécute et
    compare leur sortie au commentaire Output. Un exemple faux fait
    échouer les tests, ce qu'un bloc de code recopié dans un document
    ne fait jamais.

Les fichiers produits vont dans docs/generated/ et sont écrasés à
chaque appel. La prose qui les présente est dans docs/annexes.tex,
écrite à la main.

Usage : python3 docs/gendoc.py  (depuis la racine du dépôt timeseries)
"""
import os
import re
import subprocess
import sys

# Les deux paquets, dans l'ordre où le guide les présente : la
# définition de NaV d'abord, la bibliothèque qui s'appuie dessus
# ensuite.
PAQUETS = [
    ("notavalue", "../notavalue", "github.com/fflamingodev/notavalue"),
    ("timeseries", ".", "usefulrisk.com/timeseries"),
]

SORTIE = "docs/generated"

# Largeur utile d'une ligne de code composée en \small dans la
# géométrie du guide : 16 cm de justification, Menlo à 10 pt. Au-delà,
# la ligne dépasse dans la marge.
LARGEUR = 74

INTITULES = {
    "CONSTANTS": "Constantes",
    "VARIABLES": "Variables",
    "FUNCTIONS": "Fonctions",
    "TYPES": "Types et méthodes",
}

ENTETE = ("%% Produit par docs/gendoc.py depuis %s.\n"
          "%% Ne pas modifier : ce fichier est écrasé à chaque « make ».\n\n")


def detabule(ligne: str) -> str:
    """Remplace les tabulations de Go par quatre espaces.

    verbatim ne rend pas une tabulation ; sans cela l'indentation du
    code disparaît à la composition.
    """
    return ligne.replace("\t", "    ")


def coupe(ligne: str) -> list[str]:
    """Coupe une signature trop longue là où Go la couperait.

    Deux endroits sont admis : après une virgule de premier niveau, et
    juste après la parenthèse qui ouvre ce niveau. On retient le
    dernier qui tienne dans la largeur, et la suite est indentée. Une
    signature sans coupe possible reste longue : mieux vaut un
    dépassement visible qu'une coupe au milieu d'un identifiant.
    """
    if len(ligne) <= LARGEUR:
        return [ligne]

    profondeur, candidate = 0, -1
    for i, c in enumerate(ligne):
        if c in "([{":
            profondeur += 1
            if profondeur == 1 and i < LARGEUR:
                candidate = i
        elif c in ")]}":
            profondeur -= 1
        elif c == "," and profondeur == 1 and i < LARGEUR:
            candidate = i
    if candidate == -1:
        return [ligne]

    reste = "        " + ligne[candidate + 1:].lstrip()
    return [ligne[:candidate + 1]] + coupe(reste)


def sans_commentaire(ligne: str) -> str:
    """Retire le commentaire de fin d'une ligne de champ.

    Dans une liste de signatures, un commentaire de fin de ligne
    n'ajoute rien et fait déborder les champs porteurs d'une étiquette
    JSON. La vérification sur les guillemets évite de couper dans une
    chaîne.
    """
    i = ligne.find(" //")
    if i == -1 or ligne.count('"', 0, i) % 2 or ligne.count("`", 0, i) % 2:
        return ligne.rstrip()
    return ligne[:i].rstrip()


def signatures(chemin: str) -> list[tuple[str, list[str]]]:
    """Extrait de « go doc -all » les déclarations, sans leur prose.

    La sortie de go doc a ceci de commode que tout ce qui est déclaré
    commence en colonne zéro, et que tout ce qui l'explique est
    indenté. Le tri est donc presque celui-là, aux blocs près : un
    struct ou un const groupé s'étend sur plusieurs lignes, et il faut
    en garder les champs.
    """
    brut = subprocess.run(["go", "doc", "-all", "."], cwd=chemin,
                          capture_output=True, text=True,
                          check=True).stdout.splitlines()

    sections: list[tuple[str, list[str]]] = []
    courante: list[str] | None = None
    bloc = None  # le délimiteur attendu pour fermer un bloc ouvert

    for ligne in brut:
        if ligne in INTITULES:
            courante = []
            sections.append((INTITULES[ligne], courante))
            continue
        if courante is None:
            continue

        if bloc is not None:
            # Dans un struct ou un const groupé : on garde les champs,
            # on laisse les commentaires, qui sont dans le godoc.
            if ligne.startswith(bloc):
                courante.append(ligne)
                bloc = None
            elif ligne.strip() and not ligne.strip().startswith("//"):
                courante.append(sans_commentaire(detabule(ligne)))
            continue

        if re.match(r"(func|type|const|var) ", ligne):
            # Un type ouvre un groupe : une ligne vide le détache du
            # précédent et de ses méthodes.
            if ligne.startswith("type ") and courante:
                courante.append("")
            if ligne.rstrip().endswith("{"):
                bloc = "}"
            elif ligne.rstrip().endswith("("):
                bloc = ")"
            courante.extend(coupe(ligne))

    return [(titre, lignes) for titre, lignes in sections if lignes]


def ecris_api(nom: str, chemin: str, importpath: str) -> None:
    lignes = [ENTETE % f"go doc -all {importpath}"]
    lignes.append("\\section{\\texttt{%s}}\n" % nom)
    lignes.append("\\begin{verbatim}\nimport %s\n\\end{verbatim}\n" %
                  (f'nav "{importpath}"' if nom == "notavalue"
                   else f'"{importpath}"'))

    for titre, corps in signatures(chemin):
        lignes.append("\\subsection{%s}\n" % titre)
        lignes.append("{\\small\n\\begin{verbatim}\n%s\n\\end{verbatim}\n}\n"
                      % "\n".join(corps).strip("\n"))

    ecris(f"api-{nom}.tex", "\n".join(lignes))


def exemples(chemin: str) -> list[tuple[str, list[str]]]:
    """Extrait les fonctions Example de example_test.go.

    Le commentaire de documentation qui les précède n'est pas repris :
    il est en anglais, comme le reste du code, et l'annexe le dit en
    français dans docs/annexes.tex. Le corps, lui, est celui que le
    compilateur voit.
    """
    source = open(os.path.join(chemin, "example_test.go")).read().splitlines()
    trouves = []
    i = 0
    while i < len(source):
        m = re.match(r"func (Example\w*)\(\) \{", source[i])
        if not m:
            i += 1
            continue
        corps = [source[i]]
        i += 1
        while i < len(source) and source[i] != "}":
            corps.append(detabule(source[i]))
            i += 1
        if i < len(source):
            corps.append("}")
            i += 1
        trouves.append((m.group(1), corps))
    return trouves


def ecris_exemples(nom: str, chemin: str, importpath: str) -> None:
    """Écrit un fichier par exemple, ne contenant que le code.

    Un fichier par exemple, et non un chapitre entier, parce que la
    prose qui présente chacun d'eux est en français et vit dans
    docs/annexes.tex. Le générateur ne produit que ce qu'il sait
    produire sans se tromper : le code lui-même.
    """
    for identifiant, corps in exemples(chemin):
        contenu = (ENTETE % f"{chemin}/example_test.go"
                   + "{\\small\n\\begin{verbatim}\n%s\n\\end{verbatim}\n}\n"
                   % "\n".join(corps))
        # Pas de souligné dans un nom de fichier : \input le lirait
        # comme un indice mathématique.
        ecris(f"exemple-{nom}-{identifiant.replace('_', '-')}.tex", contenu)


def ecris(fichier: str, contenu: str) -> None:
    os.makedirs(SORTIE, exist_ok=True)
    chemin = os.path.join(SORTIE, fichier)
    open(chemin, "w").write(contenu)
    print(f"→ {chemin}")


def main() -> int:
    if not os.path.isdir("docs"):
        print("à lancer depuis la racine du dépôt", file=sys.stderr)
        return 1
    for nom, chemin, importpath in PAQUETS:
        ecris_api(nom, chemin, importpath)
        ecris_exemples(nom, chemin, importpath)
    return 0


if __name__ == "__main__":
    sys.exit(main())
