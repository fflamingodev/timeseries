<!-- Fichier produit par « make guide-fr-md » depuis docs/guide-fr.tex.
     Les modifications faites ici seront perdues à la prochaine génération. -->

# Guide de la bibliothèque *timeseries*

**Séries temporelles irrégulières, traitement de masse sous *high availability*, séries à faible contenu informatif** — Frédéric Flament

```go
import "usefulrisk.com/timeseries"
```

*Ce document est le guide de la bibliothèque. La référence des
interfaces, avec les exemples exécutables, est sur
[pkg.go.dev](https://pkg.go.dev/usefulrisk.com/timeseries) ; la version composée en PDF,
avec sa pagination et sa bibliographie, se construit par
`make guide-fr`.*

*La bibliothèque `timeseries` traite des séries de mesures matérielles, caractérisées par des intervalles irréguliers et des trous là où la chaîne de transmission est rompue. Elle nettoie, régularise et compacte — sans que des relevés absents ou incomplets ne cassent la série.*

Les trous dans les relevés sont traités par NaN-boxing : l’absence (Not-a-Value) est un flottant ordinaire. Une série traverse sans convention supplémentaire tout code qui accepte un `[]float64`, alors que les solutions usuelles — structure à champ booléen, pointeur nullable, valeur sentinelle — imposent une extraction avant chaque calcul, ou se contournent au risque de produire des erreurs silencieuses. Le procédé est éprouvé ailleurs : Prometheus distingue de la même façon, par la charge utile d’un NaN, une série arrêtée d’une erreur de calcul (§2.8.4).

Les mesures tiennent dans un tableau de flottants sans pointeur, les agrégats se lisent en une passe, et le traitement par enregistrement reste constant tant que la série tient en mémoire vive. Des mesures de performances et les variantes écartées figurent au chapitre 13.

La bibliothèque réduit sans perte les séries à faible contenu informatif — relevés à variabilité faible —, avec redéploiement.

Écrite en Go pour le rapport entre simplicité d’écriture/lecture et la performance.

## Table des matières

- [1 Introduction](#1-introduction)
- [2 NaV pour « Not a Value » : stratégie pour les données manquantes](#2-nav-pour--not-a-value---stratégie-pour-les-données-manquantes)
  - [2.1 Introduction](#21-introduction)
  - [2.2 Définition de NaV](#22-définition-de-nav)
  - [2.3 Distinction entre NaV et NaN](#23-distinction-entre-nav-et-nan)
  - [2.4 Comportement attendu d’un trou](#24-comportement-attendu-dun-trou)
  - [2.5 Règles de comportement](#25-règles-de-comportement)
  - [2.6 NaN-boxing en détail](#26-nan-boxing-en-détail)
    - [2.6.1 Structure d’un flottant](#261-structure-dun-flottant)
    - [2.6.2 Silencieux et signalant](#262-silencieux-et-signalant)
    - [2.6.3 Mise en œuvre dans la bibliothèque](#263-mise-en-œuvre-dans-la-bibliothèque)
    - [2.6.4 Contreparties](#264-contreparties)
  - [2.7 Rejet de la structure à champ booléen](#27-rejet-de-la-structure-à-champ-booléen)
    - [2.7.1 Doublement de la mémoire](#271-doublement-de-la-mémoire)
    - [2.7.2 Incompatibilité avec l’écosystème](#272-incompatibilité-avec-lécosystème)
    - [2.7.3 Traitement répété à chaque calcul](#273-traitement-répété-à-chaque-calcul)
    - [2.7.4 Contournement possible](#274-contournement-possible)
    - [2.7.5 Pointeur nullable et ramasse-miettes](#275-pointeur-nullable-et-ramasse-miettes)
    - [2.7.6 Piège de la valeur sentinelle](#276-piège-de-la-valeur-sentinelle)
    - [2.7.7 Tableau comparatif](#277-tableau-comparatif)
  - [2.8 Le traitement de l’absence dans quelques bases de données courantes](#28-le-traitement-de-labsence-dans-quelques-bases-de-données-courantes)
    - [2.8.1 Sémantique : deux régimes](#281-sémantique--deux-régimes)
    - [2.8.2 Point de divergence](#282-point-de-divergence)
    - [2.8.3 Stockage du marqueur](#283-stockage-du-marqueur)
    - [2.8.4 Bases de séries temporelles](#284-bases-de-séries-temporelles)
    - [2.8.5 Passage du moteur à Go](#285-passage-du-moteur-à-go)
  - [2.9 Avertissement](#29-avertissement)
- [3 Temps](#3-temps)
  - [3.1 Stockage d’un instant](#31-stockage-dun-instant)
  - [3.2 Temps Unix, sous les horodatages](#32-temps-unix-sous-les-horodatages)
  - [3.3 RFC 3339, forme écrite](#33-rfc-3339-forme-écrite)
  - [3.4 Durée qui n’existe pas](#34-durée-qui-nexiste-pas)
  - [3.5 Deux conventions différentes](#35-deux-conventions-différentes)
  - [3.6 Alignement d’une grille régulière sur l’UTC](#36-alignement-dune-grille-régulière-sur-lutc)
  - [3.7 Périodes calendaires: durées irrégulières](#37-périodes-calendaires-durées-irrégulières)
  - [3.8 Précision](#38-précision)
- [4 Série](#4-série)
  - [4.1 Types](#41-types)
  - [4.2 Invariant](#42-invariant)
  - [4.3 Chargement et son coût](#43-chargement-et-son-coût)
  - [4.4 Alimenter les agrégats sans allouer](#44-alimenter-les-agrégats-sans-allouer)
- [5 Statistiques de base](#5-statistiques-de-base)
- [6 Nettoyage](#6-nettoyage)
  - [6.1 Définition d’un rejet](#61-définition-dun-rejet)
  - [6.2 Valeurs jamais rejetées](#62-valeurs-jamais-rejetées)
  - [6.3 Quatre méthodes](#63-quatre-méthodes)
- [7 Régularisation](#7-régularisation)
  - [7.1 Grille](#71-grille)
  - [7.2 Tolérance](#72-tolérance)
  - [7.3 Agrégateurs](#73-agrégateurs)
- [8 Regroupement calendaire](#8-regroupement-calendaire)
- [9 Compression et restitution](#9-compression-et-restitution)
  - [9.1 Reduce](#91-reduce)
  - [9.2 Expand](#92-expand)
  - [9.3 MarkSilences](#93-marksilences)
- [10 Interpolation](#10-interpolation)
  - [10.1 Sur le temps, pas sur les rangs](#101-sur-le-temps-pas-sur-les-rangs)
  - [10.2 Sept méthodes](#102-sept-méthodes)
  - [10.3 Seulement les trous, et seulement les courts](#103-seulement-les-trous-et-seulement-les-courts)
- [11 Conteneurs](#11-conteneurs)
- [12 Sorties](#12-sorties)
  - [12.1 Terminal](#121-terminal)
  - [12.2 JSON](#122-json)
- [13 Performances, mesurées](#13-performances-mesurées)
- [14 Migration depuis la version précédente](#14-migration-depuis-la-version-précédente)
- [15 Traçabilité des décisions](#15-traçabilité-des-décisions)
- [A Exemples d’utilisation](#a-exemples-dutilisation)
  - [A.1 `notavalue`](#a1-notavalue)
    - [A.1.1 Les trois règles sur un même calcul](#a11-les-trois-règles-sur-un-même-calcul)
    - [A.1.2 L’asymétrie de l’addition](#a12-lasymétrie-de-laddition)
    - [A.1.3 Choisir la première source disponible](#a13-choisir-la-première-source-disponible)
    - [A.1.4 Ce que vaut un résultat](#a14-ce-que-vaut-un-résultat)
    - [A.1.5 Garder la distinction à l’écran](#a15-garder-la-distinction-à-lécran)
    - [A.1.6 Une médiane qui n’invente pas de valeur](#a16-une-médiane-qui-ninvente-pas-de-valeur)
  - [A.2 `timeseries`](#a2-timeseries)
    - [A.2.1 Le parcours complet](#a21-le-parcours-complet)
    - [A.2.2 Un trou et une erreur ne sont pas le même accident](#a22-un-trou-et-une-erreur-ne-sont-pas-le-même-accident)
    - [A.2.3 Nettoyer, et garder ce qu’on a rejeté](#a23-nettoyer-et-garder-ce-quon-a-rejeté)
    - [A.2.4 Un enregistreur qui dérive](#a24-un-enregistreur-qui-dérive)
    - [A.2.5 Combler un silence court, pas une panne](#a25-combler-un-silence-court-pas-une-panne)
    - [A.2.6 Comprimer un signal d’état, puis le restituer](#a26-comprimer-un-signal-détat-puis-le-restituer)
    - [A.2.7 Parcourir un conteneur sans allouer](#a27-parcourir-un-conteneur-sans-allouer)
- [B Référence des interfaces](#b-référence-des-interfaces)
  - [B.1 `notavalue`](#b1-notavalue)
    - [B.1.1 Variables](#b11-variables)
    - [B.1.2 Fonctions](#b12-fonctions)
  - [B.2 `timeseries`](#b2-timeseries)
    - [B.2.1 Constantes](#b21-constantes)
    - [B.2.2 Variables](#b22-variables)
    - [B.2.3 Fonctions](#b23-fonctions)
    - [B.2.4 Types et méthodes](#b24-types-et-méthodes)
- [Bibliographie](#bibliographie)

## 1 Introduction

`timeseries` est une bibliothèque Go qui traite des séries de relevés mesurés. Elle répond à quatre questions.

**Que faire d’un relevé qui n’existe pas ?** Un trou n’est ni un zéro, ni une erreur, ni une ligne à supprimer. La bibliothèque le représente par un **NaV**, un NaN porteur d’un repère, que les calculs ignorent au lieu de s’y arrêter — là où un NaN ordinaire, réservé aux erreurs de calcul, continue de se propager. Une moyenne mensuelle survit à un jour manquant ; elle ne survit pas à une division par zéro, et c’est la différence qu’il fallait pouvoir exprimer.

**Que faire d’un instrument qui n’est pas un métronome ?** Les relevés annoncés « toutes les heures » arrivent à 00:57, 02:03, 03:00. La régularisation les repose sur une grille de pas fixe, avec une tolérance pour les enregistreurs qui dérivent, et des trous là où aucun relevé n’est arrivé. Deux séries traitées au même pas deviennent alors comparables point à point, condition de tout calcul qui les met en relation.

**Que faire d’un signal qui ne dit presque rien ?** Une porte, une consigne, un état, un compteur : la plupart des relevés n’apportent aucune information, puisque la valeur n’a pas bougé. La bibliothèque les réduit à leurs changements — le premier relevé, le dernier, et ce qui s’est passé entre les deux — et sait les reconstituer à la demande. Une bande morte permet de n’enregistrer qu’au-delà d’un écart donné, avec une perte bornée et connue. Et comme un silence prolongé se confond, après réduction, avec une valeur qui se maintient, `MarkSilences` le marque comme un trou avant que l’information ne disparaisse.

**Que faire quand il y en a beaucoup ?** Un capteur ne pose pas de problème ; mille capteurs interrogés sur un an en posent. Les agrégats parcourent donc leurs données une seule fois et n’allouent rien ; le chargement d’un lot trie une fois, en $`n\log n`$, là où l’insertion point par point est quadratique — deux ordres de grandeur d’écart sur un million de relevés en désordre ; et un tampon réutilisable permet de boucler sur des centaines de séries sans qu’une allocation ait lieu. Chacun de ces choix est mesuré, les variantes écartées sont conservées dans le dépôt avec leurs chiffres, et les mesures sont relatives à la machine d’essai : ce sont les rapports qui se transposent, pas les durées.

Le reste du guide détaille ces quatre réponses, les conventions de temps qui les sous-tendent, et ce que chaque opération demande en mémoire, en allocations et en parcours.

## 2 NaV pour « Not a Value » : stratégie pour les données manquantes

### 2.1 Introduction

Comment savoir qu’une donnée est manquante ? Ou plutôt, comment caractériser, dans les calculs, dans les rapports, et dans le déclenchement d’action, une donnée qui n’est pas là ? Une solution courante est de déclencher une action à partir d’une absence de donnée pendant une durée fixée à l’avance. Si on examine les solutions apportées dans les logiciels courants, nous avons opté pour une solution utilisant le *NaN-boxing*.

### 2.2 Définition de NaV

La norme IEEE 754 [1], qui régit les nombres flottants, laisse libre le contenu de la mantisse d’un NaN : n’importe quelle valeur non nulle en fait un NaN valide. Nous utilisons cette liberté pour introduire un nombre particulier, le **NaV**, un NaN silencieux dont le bit 48 de la mantisse est allumé. Il reste un `float64` ordinaire : un `[]float64` n’a besoin d’aucun masque parallèle, et `math.IsNaN` le reconnaît toujours, si bien que le code déjà écrit pour se prémunir des NaN se prémunit aussi des NaV.

Dans tout ce guide, un **trou** désigne un relevé manquant, c’est-à-dire un NaV : un instant où la mesure aurait dû avoir lieu et n’a pas eu lieu.

<a id="tab:origines"></a>

|  | Signification | Exemple | Traitement |
|:---|:---|:---|:---|
| **NaV** | Rien n’a été mesuré | Capteur hors service, ligne absente, point rejeté | Ignoré |
| **NaN** | Un calcul a cassé | `0/0` en amont, `log(-1)` | Se propage |

**Table 2.1** — Origine d’un non-nombre et traitement qui en découle

### 2.3 Distinction entre NaV et NaN

- Le NaN a été créé pour les erreurs de calcul, une donnée manquante n’est pas une erreur de calcul

- Une donnée manquante ne doit pas nécessairement se propager dans tous les calculs subséquents, or les NaN propagent l’indication de l’erreur de calcul

### 2.4 Comportement attendu d’un trou

1.  **Un trou n’arrête jamais un calcul.** Il est ignoré par les agrégats et neutre dans l’addition.

2.  **Une erreur se propage toujours.** Quand un trou et une erreur se rencontrent, l’erreur l’emporte.

3.  **Quand il ne reste rien à calculer, le résultat est un trou** — jamais zéro.

La troisième règle appelle un point d’attention. La moyenne d’une série vide vaut NaV, pas 0. Une moyenne nulle est une affirmation sur les données : elle dit que les relevés se compensent, ou, plus simplement, que la température en Celsius était au point de gel. Sur une série vide, il n’y a rien à affirmer, et répondre zéro serait une invention qu’aucun lecteur ultérieur ne peut détecter.

### 2.5 Règles de comportement

Les conséquences ne sont pas nécessairement triviales:

<a id="tab:regles"></a>

| Situation | Résultat | Pourquoi |
|:---|:---|:---|
| Moyenne de `[1, 2, NaV, 3]` | `2` | Le trou est ignoré ; le diviseur vaut 3, pas 4 |
| Moyenne de `[1, 2, NaN, 3]` | `NaN` | Une erreur en amont doit rester visible |
| Moyenne de `[NaV, NaV]` | `NaV` | Rien à dire |
| `Sub(5, NaV)` | `NaV` | Une différence exige ses deux termes ; rendre 5 reviendrait à lire le trou comme un zéro |
| `Add(NaV, 5)` | `5` | Une somme tolère un terme absent |
| `Mul(NaV, 0)` | `NaV` | Une quantité inconnue de quelque chose reste inconnue |
| Variation après un trou | `NaV` | Un relevé de 14 après un trou n’est pas une hausse de 14 |
| Point rejeté comme aberrant | `NaV` | Il a été mesuré mais n’est pas cru : absent, pas cassé |
| Interpoler un `NaN` | laissé tel quel | Recouvrir une erreur d’un nombre plausible est la façon dont un bug cesse d’être remarqué |

**Table 2.2** — Règles de comportement, cas par cas

L’asymétrie entre `Add` et `Sub` peut provoquer un haussement de sourcils. La logique est la suivante: une somme accumule des termes indépendants, et un terme absent laisse les autres intacts — c’est ce qui permet à une moyenne mensuelle de survivre à un jour manquant. Une différence compare deux valeurs précises ; si l’une est inconnue, la différence l’est aussi, et toute autre réponse fabrique une variation que personne n’a mesurée.

Ces règles ne sont pas une invention locale. SQL les applique depuis toujours : `sum` « computes the sum of the non-null input values », et « sum of no rows returns null, not zero as one might expect » [2]. Un jour manquant n’empêche pas la somme du mois, et un mois sans aucun relevé ne vaut pas zéro. C’est mot pour mot la première et la troisième règle. §2.8 détaille ce que les bases de données font de l’absence, et le seul point où cette bibliothèque s’en écarte.

### 2.6 NaN-boxing en détail

Le NaN-boxing consiste à ranger de l’information **à l’intérieur** d’un nombre flottant, dans les bits que la norme IEEE 754 laisse libres. Pour comprendre où ils sont, il faut ouvrir un `float64`.

#### 2.6.1 Structure d’un flottant

Un `float64` occupe 64 bits, répartis par la norme IEEE-754 en trois champs :

     ┌─┬───────────┬────────────────────────────────────────────────────┐
     │S│  exposant │                     mantisse                       │
     └─┴───────────┴────────────────────────────────────────────────────┘
      1     11 bits                      52 bits

La valeur ordinaire se lit « mantisse × 2^exposant », avec le signe devant. Mais la norme réserve une configuration : **quand les 11 bits d’exposant valent tous 1**, ce n’est plus un nombre.

- Si la mantisse vaut zéro, c’est l’infini, positif ou négatif selon le signe.

- Si la mantisse vaut autre chose que zéro, c’est un **NaN**.

La norme ne dit pas *quelle* valeur la mantisse doit prendre. N’importe laquelle, pourvu qu’elle ne soit pas nulle, fait un NaN parfaitement valide. Il y a 2⁵² − 1 mantisses possibles, deux signes, soit **environ neuf millions de milliards de configurations qui sont toutes des NaN** — dont la moitié, celles dites silencieuses, sont utilisables sans risque. Toutes indiscernables pour l’arithmétique, et toutes perdues si personne ne s’en sert.

#### 2.6.2 Silencieux et signalant

La norme distingue encore deux familles, par le bit de poids fort de la mantisse :

- **NaN silencieux** (bit à 1) : il traverse les calculs sans bruit. C’est celui que rend `math.NaN()`, et celui que produit toute opération invalide sur un processeur courant.

- **NaN signalant** (bit à 0) : il est censé déclencher une exception matérielle. En pratique, Go ne l’utilise pas, et la plupart des environnements le convertissent en silencieux dès la première opération. On ne s’en sert pas ici.

Il reste donc **51 bits libres** dans la mantisse d’un NaN silencieux. C’est de la place perdue, que rien n’utilise.

#### 2.6.3 Mise en œuvre dans la bibliothèque

`NaV` est un NaN silencieux dont **un** de ces bits libres est allumé :

    math.NaN()  : 0 11111111111 1000000000000000000000000000000000000000000000000000
    NaV         : 0 11111111111 1000000001000000000000000000000000000000000000000000
                                ↑        ↑
                                │        └── le repère NaV (bit 48)
                                └── le bit « silencieux »

Le test tient en une ligne : c’est un NaN, **et** le bit de repère est allumé.

    func IsNaV(x float64) bool {
        return math.IsNaN(x) && math.Float64bits(x)&navTag != 0
    }

Quatre conséquences en découlent, et ce sont elles qui justifient la technique :

1.  **Aucun surcoût mémoire.** Un NaV est un `float64`. Un tableau de mesures reste un `[]float64`, sans tableau compagnon.

2.  **Le code existant continue de fonctionner.** `math.IsNaN(NaV)` est vrai, donc tout code déjà écrit pour se prémunir des NaN se prémunit aussi des NaV. Rien à recompiler, rien à auditer.

3.  **La distinction survit au transport binaire.** Copier, sérialiser en binaire, passer par `math.Float64bits` : le repère voyage avec la valeur, puisqu’il *est* la valeur.

4.  **Il reste 50 bits libres.** On pourrait un jour distinguer « jamais mesuré », « rejeté comme aberrant », « interpolé », sans rien casser de l’existant.

#### 2.6.4 Contreparties

- **L’arithmétique doit passer par les fonctions du paquet.** Ce que devient le repère à travers un `-` ordinaire dépend du processeur (§2.9). C’est la contrainte principale.

- **La sérialisation texte perd le repère.** JSON n’a pas de NaN : tout non-nombre devient `null`, et la distinction ne subsiste que dans les compteurs (§12.2).

- **C’est une technique peu connue.** Un relecteur qui découvre la bibliothèque doit d’abord comprendre ce qu’il lit — ce chapitre existe pour ça.

- **Cela ne vaut que pour les flottants.** Le procédé tient aux bits libres du NaN d’IEEE 754 [1] ; un entier, un booléen, une chaîne n’ont pas d’encodage inutilisé à détourner. La bibliothèque le constate dès qu’elle quitte le flottant : une durée manquante est une valeur sentinelle, `NaDuration`, et non un repère logé dans des bits libres (chapitre 3).

Marquer l’absence dans n’importe quel type demande donc un marqueur en dehors de la valeur, sous l’une de deux formes : un champ booléen accolé à chaque valeur, dont la section suivante détaille le prix, ou un bit par valeur dans un masque séparé. C’est la voie des bases de données, et §2.8 dit comment elles la suivent.

### 2.7 Rejet de la structure à champ booléen

C’est la solution la plus répandue :

    type Valued struct {
        Value float64
        Valid bool
    }

C’est ce que font `sql.NullFloat64` en Go, `Option<f64>` en Rust, `double?` en C#, `Optional<Double>` en Java. Six raisons de ne pas l’avoir reprise.

#### 2.7.1 Doublement de la mémoire

    float64                       :  8 octets
    struct{ float64; bool }       : 16 octets

Le booléen n’occupe qu’un octet, mais l’alignement en impose huit : sept octets sont perdus par point. Sur un million de mesures, **8 Mo deviennent 16 Mo** ; sur dix millions, 80 deviennent 160.

Un masque parallèle `[]bool` fait mieux — 9 Mo — mais au prix d’un second tableau à transporter, à découper et à trier en même temps que le premier, et qu’on oublie à la première refonte.

#### 2.7.2 Incompatibilité avec l’écosystème

Tout ce qui calcule sur des flottants, en Go, attend un `[]float64`: gonum, les transformées de Fourier, les bibliothèques de statistiques, et `notavalue` lui-même.

Avec une structure, chaque appel exige d’abord une extraction — une boucle et une allocation proportionnelles à la série :

<a id="tab:extraction"></a>

| Moyenne sur un million de points          | Temps  | Allocation         |
|:------------------------------------------|:-------|:-------------------|
| Structure, puis extraction en `[]float64` | 368 µs | **8 Mo par appel** |
| NaN-boxing, tableau passé tel quel        | µs     | **aucune**         |

**Table 2.3** — Moyenne sur un million de points : extraction contre passage direct

Deux fois plus lent, et 8 Mo alloués à chaque appel. Sur un traitement qui enchaîne moyenne, médiane, écart-type et percentiles, la facture se paie quatre fois. Les durées absolues valent pour la machine d’essai décrite au chapitre 13 ; c’est le rapport entre les deux lignes qui compte.

#### 2.7.3 Traitement répété à chaque calcul

Chaque fonction qui travaille sur les mesures doit connaître la structure et tester le booléen. Le traitement des absences se répète partout au lieu d’être porté par la valeur elle-même.

#### 2.7.4 Contournement possible

Le compilateur n’oblige jamais à lire `Valid`. Rien n’empêche d’écrire `v.Value` sur un relevé absent : on obtient zéro, un zéro qui a l’air d’une mesure et qui traverse tout le calcul sans laisser de trace. L’oubli n’est signalé ni à la compilation ni à l’exécution, et il se glisse dans la première fonction écrite un jour de fatigue.

Un trou, lui, se défend tout seul. Il n’existe aucune façon de lire « la valeur derrière le NaV » : la valeur *est* le NaV, et toute opération qui l’ignore le propage ou l’écarte selon les règles du paquet. Le mauvais usage n’est pas rendu difficile, il est rendu impossible.

#### 2.7.5 Pointeur nullable et ramasse-miettes

`*float64`, avec `nil` pour l’absence, semble élégant. Mais chaque point devient un pointeur de 8 octets **plus** la valeur pointée quelque part ailleurs, et surtout : un tableau d’un million de pointeurs est parcouru par le ramasse-miettes à chaque cycle, alors qu’un `[]float64` ne contient aucun pointeur et lui reste totalement invisible.

#### 2.7.6 Piège de la valeur sentinelle

Coder l’absence par −999, ou par 0, est la solution la plus ancienne. Elle fonctionne jusqu’au jour où une vraie mesure vaut −999 — et ce jour arrive. Le NaV, lui, ne peut pas être confondu avec une mesure : aucune opération sur des nombres réels ne le produit.

#### 2.7.7 Tableau comparatif

<a id="tab:representations"></a>

| Solution | Mémoire par point | Compatible `[]float64` | Invisible au GC | Confusion possible |
|:---|:---|:---|:---|:---|
| **NaN-boxing** | o | oui | oui | non |
| `struct{float64; bool}` | o | non | oui | non |
| `[]float64` + `[]bool` | o | oui, mais le masque suit à part | oui | non |
| `*float64` | o + la valeur | non | **non** | non |
| Sentinelle −999 | o | oui | oui | **oui** |

**Table 2.4** — Représentations de l’absence dans une série de flottants

La vitesse de parcours, elle, ne départage pas : les quatre premières tournent entre 0,6 et 0,8 ms par million de points, et l’écart est dominé par le reste du calcul. **Ce qui décide, c’est la mémoire et la compatibilité**, pas les nanosecondes.

### 2.8 Le traitement de l’absence dans quelques bases de données courantes

La question est ancienne et elle a été tranchée ailleurs, à grande échelle. Un moteur de base de données doit accepter l’absence dans n’importe quel type, pas seulement dans un flottant, et il doit répondre à deux questions distinctes : ce qu’un calcul fait d’une valeur absente, et où le marqueur se loge. Les réponses au premier point sont remarquablement proches de celles de cette bibliothèque ; celles du second lui sont inaccessibles.

#### 2.8.1 Sémantique : deux régimes

SQL sépare les opérateurs scalaires des agrégats, et les traite en sens inverse.

Un opérateur scalaire propage : `NULL + 5` vaut NULL, et toute comparaison avec NULL vaut *unknown* plutôt que vrai ou faux — d’où la logique à trois valeurs et le `IS NULL`, qui existe parce que `= NULL` ne peut rien affirmer.

Un agrégat saute : `sum` additionne les valeurs non nulles, `avg` divise par leur nombre et non par celui des lignes, `count(colonne)` compte les valeurs renseignées là où `count(*)` compte les lignes. Et une somme sans aucune ligne rend NULL, pas zéro [2]. C’est la troisième règle de ce chapitre, à l’identique : quand il n’y a rien à calculer, le résultat est l’absence.

Reste `coalesce`, que SQL fournit pour qui veut le zéro, et qui porte le même nom que la fonction correspondante de `notavalue`.

#### 2.8.2 Point de divergence

`Add(NaV, 5)` rend 5, là où `NULL + 5` vaut NULL. La divergence est apparente : `Add` n’est pas le `+` de SQL, c’est sa primitive d’accumulation, et l’accumulateur de SQL est `sum`, qui saute les absents exactement de la même façon. Le `+` scalaire de SQL a pour équivalent `Sub`, `Mul`, `Div`, qui propagent tous l’absence.

Une chose en revanche n’a pas d’équivalent : la deuxième règle. Un calcul rompu n’y produit pas une valeur qui se propage, il lève une erreur et la transaction s’arrête — `0/0` est une *division by zero*, non un NaN silencieux. SQL peut se le permettre : il a une transaction à annuler. Une bibliothèque appelée au milieu d’un traitement de masse n’a pas ce recours, et le NaN qui se propage est ce qui tient lieu d’erreur transportable.

#### 2.8.3 Stockage du marqueur

**PostgreSQL** range le marqueur hors de la valeur, et non dans un booléen accolé à elle. Chaque ligne porte, juste après son en-tête fixe, une *null bitmap* d’un bit par colonne — 1 pour renseigné, 0 pour NULL — et cette carte n’est présente que si l’en-tête signale au moins un NULL [3]. Une colonne NULL n’occupe alors aucun octet pour sa valeur : le bit remplace la donnée au lieu de s’y ajouter.

**SQLite** n’a pas de carte, parce qu’il n’a pas de types de colonne fixes : l’en-tête de chaque enregistrement énumère le type de chaque valeur, et l’absence est l’un de ces types. Le type sérialisé 0 signifie « la valeur est NULL » et occupe zéro octet de contenu [4]. Le marqueur ne coûte donc rien de plus que ce que toute valeur paie déjà, et fait l’économie du contenu.

**Apache Arrow**, le format en mémoire derrière une bonne part des outils analytiques actuels, tient un tampon séparé : un bit par emplacement, 1 pour renseigné, 0 pour absent. Avec une réserve que la spécification énonce elle-même — « a null value may occupy a non-empty memory space in the data buffer », et le contenu de cet emplacement est alors indéfini [5]. Sur un type de largeur fixe, un absent paie donc le bit *et* la place.

<a id="tab:marqueurs"></a>

| **Système** | **Où est le marqueur** | **Ce que coûte une valeur absente** |
|:---|:---|:---|
| NaN-boxing | dans la valeur | rien : le flottant est le marqueur |
| PostgreSQL | 1 bit par colonne, en-tête de ligne | 1 bit, et aucun octet de contenu |
| SQLite | type sérialisé par valeur, en-tête d’enregistrement | rien de plus qu’une valeur présente, et aucun contenu |
| Apache Arrow | 1 bit par valeur, tampon séparé | 1 bit *et* l’emplacement, réservé |
| `struct{float64; bool}` | accolé à la valeur | 8 octets, par alignement |

**Table 2.5** — Emplacement du marqueur d’absence selon le système

Deux des trois font mieux que tout ce que la table [2.4](#tab:representations) avait examiné.

#### 2.8.4 Bases de séries temporelles

Ce sont les plus proches parentes du sujet : elles ne stockent que des couples instant-mesure, et rencontrent donc le problème de ce guide dans sa forme pure. Aucune des deux n’a de NULL au sens relationnel. Un trou y est l’absence de relevé, pas un relevé marqué.

**Prometheus** a pourtant eu besoin de dire autre chose que « pas de relevé » : quand une cible disparaît, il faut distinguer « la série s’est arrêtée » de « rien n’est encore arrivé ». Sans quoi la dernière valeur connue continue d’être rendue pendant la fenêtre de rattrapage, et un graphique montre une machine éteinte comme si elle fonctionnait encore. La série est donc marquée périmée, et « if a query is evaluated at a sampling timestamp after a time series is marked as stale, then no value is returned for that time series » [6].

Ce qui nous intéresse est la façon dont ce marqueur est représenté. Elle est celle de ce chapitre [7] :

    // NormalNaN is a quiet NaN. This is also math.NaN().
    NormalNaN uint64 = 0x7ff8000000000001

    // StaleNaN is a signaling NaN, due to the MSB of the
    // mantissa being 0.
    StaleNaN uint64 = 0x7ff0000000000002

    func IsStaleNaN(v float64) bool {
        return math.Float64bits(v) == StaleNaN
    }

Deux NaN distingués par leur charge utile, l’un pour l’erreur de calcul, l’autre pour l’absence, et un prédicat pour les séparer. C’est exactement `IsNaV` et `IsStdNaN`, atteint séparément et déployé à très grande échelle. Le procédé décrit ici n’est donc pas une curiosité locale.

Deux différences méritent d’être relevées. Prometheus prend un NaN *signalant* — bit de poids fort de la mantisse à zéro, cf. §2.6.2 — là où `notavalue` prend un NaN silencieux marqué. Il peut se le permettre parce que son marqueur n’entre jamais dans un calcul : il est filtré à la lecture. Un NaV, lui, traverse `Add`, `Sub` et les agrégats, et doit donc être silencieux. Et Prometheus compare la totalité des bits, ce qui ferme la porte à d’autres charges utiles ; le masque sur un seul bit la laisse ouverte.

**InfluxDB** ne marque rien dans le stockage : un champ qui n’a pas été écrit n’existe pas à cet instant. En revanche il tranche à la lecture, et la clause `fill()` d’InfluxQL énumère les mêmes choix que la table [10.1](#tab:interpolation) — `null`, `previous`, `linear`, `none`, ou une valeur numérique [8]. Deux d’entre eux se lisent comme un avertissement : `fill(none)` supprime l’horodatage avec la valeur, donc le trou lui-même, ce que `Regularize` refuse de faire ; et `fill(0)` est précisément le geste contre lequel cette bibliothèque existe.

Le point remarquable est ailleurs : le comportement par défaut d’InfluxDB est `fill(null)`. Sur une fenêtre sans données, il rend un horodatage et une absence, pas un zéro. La même décision que celle de ce chapitre, prise par les deux moteurs de séries temporelles les plus répandus.

#### 2.8.5 Passage du moteur à Go

La représentation économique du moteur ne survit pas au transport. Un pilote qui rend une colonne `double precision` nullable ne peut pas la déposer dans un `float64` : la bibliothèque standard refuse, et le message ne laisse pas de doute — `converting NULL to float64 is unsupported`. Il faut élargir le type, et `database/sql` le fait exactement sous la forme que §2.7 écarte [9] :

    type NullFloat64 struct {
        Float64 float64
        Valid   bool
    }

La forme générique ajoutée depuis, `sql.Null[T]`, a la même structure : une valeur, un booléen. Le bit de la *null bitmap* devient donc un champ de huit octets dès la frontière, alignement compris, et le tableau compact du moteur arrive en Go sous forme de tranche de structures — avec les six conséquences déjà énumérées.

C’est là que se mesure l’intérêt du procédé. Aucune des trois représentations de la table [2.5](#tab:marqueurs) n’est à la portée d’une bibliothèque : un moteur maîtrise sa représentation du premier octet sur disque au dernier en mémoire, et peut donc décider que l’absence vit à côté de la donnée. `timeseries` rend des `[]float64` à du code qu’elle ne connaît pas. C’est cette tranche-là, sans carte à transporter à côté d’elle, qui est la contrainte — et le NaN-boxing est ce qui permet de la tenir, au point précis où `database/sql` doit renoncer.

### 2.9 Avertissement

Il ne faut pas utiliser les opérateurs ordinaires sur des valeurs susceptibles d’être absentes. La charge utile que porte un NaN résultat est laissée au processeur, et les processeurs ne s’accordent pas : sur certains, `NaV - 5` ressort encore marqué comme NaV, sur d’autres le repère est perdu. Passez par les `Add`, `Sub`, `Mul`, `Div` et les agrégats de `notavalue`, qui font du résultat une propriété du code et non de la machine qui l’exécute.

## 3 Temps

### 3.1 Stockage d’un instant

Les horodatages sont des [`time.Time`](https://pkg.go.dev/time#Time), tels que définis par la bibliothèque standard de Go [10]. Celle-ci a été déterminante dans le choix du langage pour le traitement des données temporelles.`time.Time` est un type bien développé en Go — fuseaux, formatage, comparaison sont dans la bibliothèque standard[^1]. Le cadre est l’utilisation de Unix Time dans toute la chaîne de traitement, le formatage en RFC3339 appliqué partout là où le passage par le texte est nécessaire.

### 3.2 Temps Unix, sous les horodatages

Sous la surface, tous les systèmes qui échangent des dates s’accordent sur une même origine : **le 1ᵉʳ janvier 1970 à 00:00:00 UTC**. Un instant s’y ramène à un seul nombre, le compte de ce qui s’est écoulé depuis — secondes, millisecondes ou nanosecondes selon la précision retenue. C’est le *temps Unix*, et c’est ce que `time.Time` manipule en interne, ce que PostgreSQL stocke, ce que transportent les capteurs.

Quelques repères pour lire un tel nombre :

<a id="tab:unix"></a>

| Nombre                      | Unité        | Instant                        |
|:----------------------------|:-------------|:-------------------------------|
| `0`                         | seconde      | 1ᵉʳ janvier 1970, 00:00:00 UTC |
| `1 000 000 000`             | secondes     | 9 septembre 2001               |
| `1 767 225 600`             | secondes     | 1ᵉʳ janvier 2026               |
| `1 767 225 600 000 000 000` | nanosecondes | le même instant                |

**Table 3.1** — Lectures du temps Unix

Trois propriétés méritent d’être connues, parce qu’elles expliquent des choix de la bibliothèque.

**Le temps Unix ne connaît pas les fuseaux.** C’est un compte depuis une origine, donc un instant absolu. Deux capteurs, l’un à Luxembourg et l’autre à Tokyo, qui mesurent au même moment produisent le même nombre. Le fuseau n’intervient qu’à l’affichage — et c’est exactement pourquoi une grille régulière s’aligne dessus (§3.6) : c’est la seule référence que deux séries partagent, où qu’elles aient été enregistrées.

**Il ignore les secondes intercalaires.** La rotation de la Terre n’est pas régulière, et l’UTC y ajoute de temps en temps une seconde — la dernière en 2016. Le temps Unix, lui, fait comme si elles n’existaient pas : une journée y compte toujours exactement 86 400 secondes. Les systèmes qui doivent rester à l’heure les absorbent en étirant imperceptiblement leur horloge sur quelques heures. Conséquence pratique : une durée calculée entre deux instants séparés par une seconde intercalaire est fausse d’une seconde, ce qui n’a d’importance que pour la métrologie fine.

**Il ne dit rien de la précision de la mesure.** Un horodatage à la nanoseconde n’implique pas que le capteur sache ce qu’il faisait à la nanoseconde près. Le nombre est exact, la mesure ne l’est pas forcément — et la bibliothèque conserve ce qu’on lui donne sans prétendre l’améliorer.

En Go, un `time.Time` porte davantage qu’un simple compte : l’instant absolu, un fuseau, et parfois une [lecture d’horloge monotone](https://pkg.go.dev/time#hdr-Monotonic_Clocks) — insensible aux changements d’heure du système, là où la lecture d’horloge murale les subit — que la bibliothèque standard utilise pour mesurer des durées. Les horodatages venus d’une base ou d’un capteur n’en ont pas ; c’est sans conséquence ici.

### 3.3 RFC 3339, forme écrite

Le temps Unix est un nombre ; il faut aussi une forme écrite, lisible par un humain et non ambiguë pour une machine. C’est la **RFC 3339**, et c’est elle que produit `ToJSON` :

    2026-01-15T14:30:00Z           ← en UTC, le « Z » pour zéro décalage
    2026-01-15T15:30:00+01:00      ← le même instant, vu de Paris
    2026-01-15T14:30:00.123456789Z ← avec ses nanosecondes

La forme est stricte, et c’est ce qui en fait la valeur : une date, un `T`, une heure, un décalage. Du plus grand au plus petit, toujours, avec des zéros de remplissage. C’est un sous-ensemble volontairement réduit de la norme ISO 8601, qui autorise elle une foule de variantes — les semaines, les durées, les dates partielles, l’omission des séparateurs — et qu’aucune implémentation ne couvre entièrement.

Trois propriétés justifient de s’y tenir :

**Le tri lexicographique est le tri chronologique.** Deux horodatages RFC 3339 exprimés dans le même décalage se comparent comme du texte ordinaire, caractère par caractère, et l’ordre obtenu est le bon. C’est ce qui permet de trier un fichier de journaux avec `sort`, ou d’indexer une colonne texte sans la convertir.

**Le décalage est obligatoire.** Un horodatage sans décalage — comme en produisent tant de bases de données et d’API — ne désigne pas un instant : `2026-01-15 14:30:00` peut être quatorze heures et demie à Paris, à Tokyo ou à New York, soit trois instants distants de plusieurs heures. La RFC 3339 [11] l’interdit. Quand une donnée arrive dans cette forme, il faut lui adjoindre le fuseau que le fournisseur sous-entend, et c’est une décision, pas une conversion.

**Mais un décalage n’est pas un fuseau.** `+01:00` dit de combien l’heure locale s’écarte de l’UTC à cet instant précis ; il ne dit pas qu’on est à Paris, ni si l’heure d’été s’appliquait. Une série sérialisée en RFC 3339 puis relue perd donc le nom de son fuseau : elle garde l’instant exact, ce qui suffit à tout calcul, mais un regroupement calendaire effectué après ce trajet retombera sur un décalage figé plutôt que sur un vrai fuseau. **Quand les journées locales comptent, regroupez avant de sérialiser**, pas après.

### 3.4 Durée qui n’existe pas

Le premier point d’une série n’a pas de prédécesseur, donc l’intervalle qui le précède n’existe pas. C’est `NaDuration`, le pendant de NaV pour le temps. Il s’affiche « NaDuration » plutôt que sous la forme absurde de −2562047h47m16s, et `IsNaDuration` le reconnaît.

### 3.5 Deux conventions différentes

<a id="tab:conventions"></a>

|  | Fenêtre | Un relevé sur la frontière | L’instant émis |
|:---|:---|:---|:---|
| `Regularize` | fermée à droite | appartient à la fenêtre qui s’y **termine** | le top |
| `Downscale*` | ouverte à gauche | **ouvre** la nouvelle journée | le dernier instant de la période |

**Table 3.2** — Conventions de fenêtre : régularisation et regroupement

`Regularize` convient au calcul : toutes ses fenêtres durent exactement la même chose, donc deux points pèsent toujours pareil. `Downscale` convient au calendrier : un relevé pris à minuit appartient à la journée qui commence, comme le dirait n’importe qui.

Les deux datent leur point à la **fin** de la période, si bien que tous deux se lisent « tout ce qui précède jusqu’ici ».

### 3.6 Alignement d’une grille régulière sur l’UTC

La grille ne s’aligne pas sur le premier relevé — ce qui rendrait deux séries incomparables — mais sur le temps absolu, c’est-à-dire sur l’UTC.

Dans un fuseau décalé d’un nombre entier d’heures, c’est invisible :

    Paris (UTC+01:00), relevé à 10:47 → fenêtre horaire à 10:00 locales

Dans un fuseau décalé d’une demi-heure, ça ne l’est pas :

    Calcutta (UTC+05:30), relevé à 10:47 → fenêtre horaire à 10:30 locales

Ce sont les mêmes instants en UTC, et c’est précisément l’objectif : deux séries régularisées au même pas tombent sur les mêmes instants, où qu’elles aient été enregistrées. Si ce sont les frontières d’heures locales qui comptent — un rapport quotidien pour une équipe sur place —, utilisez `DownscaleDaily`, qui travaille dans le calendrier, ou décalez les horodatages avant de régulariser.

### 3.7 Périodes calendaires: durées irrégulières

Les frontières du calendrier n’ont de sens qu’en un lieu : la famille `Downscale` les calcule donc dans le fuseau du premier relevé de la série.

Elle traite les deux jours de l’année où les horloges changent — 23 heures au printemps, 25 à l’automne — et les fuseaux où **minuit n’existe pas** : à Santiago, La Havane et aux Açores, les horloges avancent à minuit, et la journée s’ouvre à 01:00. Bâtir une période sur un minuit qui n’a jamais eu lieu la fermerait une heure trop tôt, et toutes les périodes suivantes avec elle.

### 3.8 Précision

Les statistiques sur les horodatages sont calculées sur des écarts au premier point, et non sur des nanosecondes depuis 1970.

Un `float64` porte 53 bits de mantisse ; un horodatage Unix actuel en nanosecondes en demande 61. Calculer sur des valeurs absolues arrondit à quelques centaines de nanosecondes — de quoi faire tomber au mauvais endroit l’instant moyen de trois relevés espacés de deux nanosecondes. Compter depuis le début de la série garde les nombres petits et le résultat exact.

## 4 Série

### 4.1 Types

    type Datum struct {                 // un relevé
        Chron time.Time
        Meas  float64
    }

    type DataUnit struct {              // un relevé placé dans une série
        Datum
        Dchron time.Duration            // temps écoulé depuis le point précédent
        Dmeas  float64                  // variation depuis le point précédent
    }

Une `TimeSeries` est une suite de `DataUnit`, plus un `ID`, un `Name` pour les humains et un `Comment` qui retrace sa provenance.

### 4.2 Invariant

**À tout instant, les points sont dans l’ordre chronologique et chaque delta s’accorde avec cet ordre.**

Aucune méthode ne trie une série ni ne recalcule ses deltas : il n’y a jamais rien à réparer. `Add` et `AddBatchData` maintiennent l’invariant en insérant — y compris quand un relevé arrive en retard, ce qui décale la fin du tableau et recalcule exactement deux deltas.

D’où le champ privé. Livrer le tableau permettrait d’ajouter hors ordre ou de trier par mesure, laissant les deltas décrire un ordre disparu. La version précédente portait pour cela un drapeau, `deltasValid`, qui prévenait que son propre état pouvait mentir. Rendre l’état impossible vaut mieux que le signaler.

La lecture passe par `Len`, `At`, `First`, `Last`, `Range`, `Meas` et `MeasTo`.

### 4.3 Chargement et son coût

<a id="tab:chargement"></a>

| Un million de relevés en désordre | Temps        |
|:----------------------------------|:-------------|
| `Add`, un par un                  | ~2,5 minutes |
| `AddBatchData`                    | ~1 seconde   |

**Table 4.1** — Chargement d’un million de relevés en désordre

`Add` décale la fin du tableau pour chaque relevé qui doit s’insérer plus tôt, ce qui est quadratique. Mesuré : 61 ms pour 20 000 points, 255 ms pour 40 000, 1 014 ms pour 80 000 — un quadruplement à chaque doublement.

`AddBatchData` ajoute tout, trie une fois et remplit les deltas en une passe : 9, 21 et 45 ms sur les mêmes lots. Quand le lot prolonge déjà la série dans l’ordre — une requête avec `ORDER BY` —, il le constate en une passe et saute le tri.

`Add` pour le relevé qui arrive seul d’un flux, `AddBatchData` pour tout ce qu’on a déjà en main.

Ces durées ont été relevées sur la machine d’essai du chapitre 13. Ce qui se transporte d’une machine à l’autre n’est pas la seconde ni les deux minutes et demie, mais le quadruplement à chaque doublement d’un côté et le doublement de l’autre.

### 4.4 Alimenter les agrégats sans allouer

    var buf []float64
    for _, ts := range all {
        buf = ts.MeasTo(buf[:0])           // aucune allocation après la première série
        fmt.Println(ts.Name, nav.Mean(buf))
    }

`Meas()` alloue un tableau neuf et reste la forme commode ; `MeasTo` remplit un tampon que vous gardez, ce qui compte dès qu’on boucle sur des centaines de séries.

## 5 Statistiques de base

`Stats()` renvoie trente champs, calculés à la demande et jamais mis en cache — un résumé ne peut donc jamais décrire une série qui a changé depuis, et deux appels s’accordent toujours.

Les noms suivent une clé : `Ch*` concernent les horodatages, `Ms*` les mesures, `DCh*` et `DMs*` les deltas, et `ChAt*` donnent l’instant auquel une autre statistique se produit.

Regroupés par la question à laquelle ils répondent :

<a id="tab:basicstats"></a>

| Question | Champs |
|:---|:---|
| Sur quoi la série s’étend-elle ? | `Chmin`, `Chmax`, avec `ValAtChmin` et `ValAtChmax` |
| Où commencent les données ? | `ChFirstUsable`, `ValAtFirstUsable` |
| Où se situent les points ? | `Chmean`, `Chmed` |
| Qu’a-t-on mesuré ? | `Msmin`, `Msmax` avec leurs instants, `Msmean`, `Msmed`, `Msstd` |
| À quelle cadence arrivent les relevés ? | `DChmin`, `DChmax` avec leurs instants, `DChmean`, `DChmed`, `DChstd` |
| De combien ça bouge ? | `DMsmin`, `DMsmax`, `DMsmean`, `DMsmed`, `DMsstd` |
| Quelle est la qualité des données ? | `Len`, `NbreOfNaN`, `NbreOfNaV` |

**Table 5.1** — Champs de `BasicStats`, par question posée

**`Chmin` face à `ChFirstUsable`.** Le premier dit où la fenêtre s’ouvre, le second où les relevés commencent. Sur un capteur qui chauffait encore, les deux diffèrent, et les confondre date la série trop tôt.

**`NbreOfNaN` moins `NbreOfNaV`** donne le nombre de véritables erreurs de calcul. Quand les deux sont égaux, tout ce qui n’est pas un nombre dans la série est un trou, et rien n’est cassé.

**`DChstd`** mesure la régularité de l’échantillonnage : proche de zéro sur un flux discipliné, il grimpe dès qu’il bégaie. C’est souvent le nombre le plus instructif du tableau, parce qu’un intervalle moyen masque complètement une dérive — dix-neuf intervalles de 56 à 66 minutes ont une moyenne d’exactement une heure.

Une statistique qui ne peut pas être calculée vaut NaV, jamais zéro. Un relevé unique n’a pas d’écart-type ; une série de trous n’a pas de moyenne.

## 6 Nettoyage

### 6.1 Définition d’un rejet

Un point rejeté par une fonction de nettoyage n’est ni une erreur ni une suppression et produit une série temporelle dédiée. Parfois c’est dans les données rejetées que se trouve l’information: fréquence des pannes, maintenance préventive, déclenchement d’alarmes.

Les relevés rejetés reviennent dans une série à part, avec leurs valeurs d’origine.

### 6.2 Valeurs jamais rejetées

- **Un trou** : il n’y avait rien à juger.

- **Une valeur cassée** : le nettoyage traite des mesures invraisemblables, et déguiser une erreur en absence revient à la perdre.

Ni l’un ni l’autre ne participent au calcul des seuils.

### 6.3 Quatre méthodes

<a id="tab:nettoyage"></a>

| Méthode | Seuil | À utiliser quand |
|:---|:---|:---|
| `RemoveOutbounds(min, max)` | Fixe | Bornes fixes: une pluie négative, une température au-dessus de l’ébullition |
| `RemovePercentileOutliers(low, high)` | Percentiles des relevés | Elimination des queues de distribution |
| `RemoveZScoreOutliers(level)` | Moyenne ± level × écart-type | Les relevés se dispersent symétriquement ; 3 est l’usage |
| `RemovePeirceOutliers()` | Critère de Peirce | Aucun seuil ne s’impose |

**Table 6.1** — Méthodes de nettoyage

Les bornes s’expriment en `float64`, avec `NaV` pour « pas de borne de ce côté » :

    ts.RemoveOutbounds(0, nav.NaV)            // rien en dessous de zéro
    ts.RemovePercentileOutliers(nav.NaV, 95)  // couper le haut seulement

**Sur les percentiles :** ils décrivent les relevés dont on dispose, donc les seuils bougent avec les données et quelque chose est toujours rejeté. C’est un outil différent des bornes fixes.

**Sur le critère de Peirce :** il ne prend aucun seuil. Il le déduit de la taille de l’échantillon, à travers une table publiée par Benjamin Peirce en 1852 [12], [13], et décide combien de relevés un échantillon de cette taille peut légitimement perdre. Il est plus sévère sur les petits échantillons, rejette au plus neuf relevés quelle que soit la taille, et — le test qui compte — ne rejette rien du tout d’une série saine. Sur l’exemple classique d’enseignement, il condamne exactement les deux relevés bas, 90 et 89.

## 7 Régularisation

### 7.1 Grille

    hourly, err := ts.Regularize(time.Hour, timeseries.AggMean)

Une série régularisée est une série dont les temps d’horodatage sont espacés régulièrement. La durée entre l’horodatage précédent et l’horodatage courant forme un intervalle semi-ouvert à gauche (fermé à droite).

Les fonctions de régularisation créent effectivement ce qui s’appelle une *série temporelle*, en ce sens que chaque mesure est séparée des autres par une durée identifiable. On va parler de moyenne quotidienne, de prix à la fin du mois, etc. Les fenêtres ferment à droite et s’alignent sur l’horloge (§3.6).

Quand l’application reçoit plusieurs mesures dans la fenêtre demandée, un choix doit être fait sur le traitement des mesures intermédiaires. Une collection d’agrégateurs a été mise en place à cet effet (§7.3, table [7.2](#tab:agregateurs)).

Une fenêtre qui n’a rien reçu devient un trou : une grille régulière a un point par pas, et un pas sans relevé reste un pas. Rien n’est émis avant le premier relevé ni après le dernier — une série ne dit rien de ce qui précède son début ni de ce qui suit sa fin.

### 7.2 Tolérance

Un enregistreur censé rendre son relevé à l’heure pile et qui le rend à 10:00:04 n’a pas manqué sa fenêtre. Sans tolérance, il tombe une fenêtre trop tard :

<a id="tab:tolerance"></a>

| Fenêtre | Sans tolérance                         | 5 minutes de tolérance |
|:--------|:---------------------------------------|:-----------------------|
| :00     | 57                                     | 57                     |
| 02:00   | **NaV**                                | 123                    |
| 03:00   | **151,5** — la moyenne de deux relevés | 180                    |
| 04:00   | **NaV**                                | 241                    |

**Table 7.1** — Effet d’une tolérance de cinq minutes

Le signal horaire devient une alternance de trous et de moyennes fausses. La tolérance doit rester inférieure au pas, sans quoi un relevé appartiendrait à deux fenêtres.

### 7.3 Agrégateurs

<a id="tab:agregateurs"></a>

| Agrégateur | Renvoie | Exemple d’utilisation |
|:---|:---|:---|
|  |  |  |
| Agrégateur | Renvoie | Exemple d’utilisation |
| `AggMean` | La moyenne de la fenêtre | Une grandeur physique |
| `AggMedian` | La médiane | Idem, quand un relevé égaré ne doit pas tirer le résultat |
| `AggMin`, `AggMax` | Les extrêmes | Mesure de composants toxiques (CO2, CO,...) |
| `AggSum` | La somme | Une grandeur qui s’accumule : pluie, énergie, événements |
| `AggFirst`, `AggLast` | Le relevé du bord, trou compris | Un signal d’état, un compteur, le prix d’un bien |
| `AggFirstUsable`, `AggLastUsable` | Le relevé du bord, en enjambant les trous | Une grandeur physique |
| `AggCountUsable` | Combien de relevés sont arrivés | Un rapport de couverture |
| `AggSlope` | La pente de la droite ajustée | Une tendance par pas |
| `AggIntegral(step)` | L’aire sous la fenêtre | Transformer un débit en quantité |

**Table 7.2** — Agrégateurs de fenêtre *(suite)*

Tous suivent la politique NaV : un trou est ignoré, une erreur se propage. Une fenêtre vide n’atteint jamais l’agrégateur — elle est émise en NaV directement.

`Aggregator("maximum")` en renvoie un par son nom, pour qu’une recette stockée en base puisse le choisir. Un nom inconnu est une erreur, jamais un repli silencieux sur la moyenne.

## 8 Regroupement calendaire

`DownscaleDaily`, `DownscaleWeekly`, `DownscaleMonthly` et `DownscaleYearly` regroupent par période du calendrier.

Une durée constante est satisfaisante d’un point de vue statistique, mais parfois surprenantes à lire sur un calendrier. Car dans le calendrier *social* auquel nous sommes habitués, rien n’est constant: à part les secondes, tous nos repères habituels (jours, mois, années) sont irréguliers. D’où la nécéssité de faire des regroupements calendaires.

- un mois dure de 28 à 31 jours — février contient environ 10 % de temps en moins que mars ;

- une année dure 365 ou 366 jours ;

- une journée dure 24 heures, sauf les deux où les horloges changent (*seconde intercalaire* - *leap second*) .

Deux points mensuels ne pèsent donc pas la même chose. Un agrégateur qui croît avec la durée de la fenêtre — `AggSum`, `AggCountUsable`, `AggIntegral` — produit des nombres qui diffèrent en partie parce que les périodes diffèrent, et février ressort plus bas pour la seule raison qu’il est plus court.

**Régulariser pour calculer, regrouper en dernier, pour montrer.**

## 9 Compression et restitution

### 9.1 Reduce

Un signal qui tient sa valeur entre deux changements — une porte, une consigne, une machine à états — stocke le même nombre encore et encore. `Reduce` garde le premier point, le dernier, et les changements entre les deux.

Ce qui compte comme changement suit la politique plutôt qu’une comparaison ordinaire, puisqu’aucun NaN n’est égal à lui-même : deux trous consécutifs ne sont pas un changement, mais entrer dans un trou ou en sortir en est un — et c’est souvent l’information la plus intéressante qu’une série enregistre.

`ReduceWithDeadband(band)` fait de même avec une tolérance : un relevé n’est gardé que s’il diffère **du dernier relevé gardé** de plus que la bande — et non du relevé précédent, si bien qu’une dérive lente est attrapée dès qu’elle a bougé de plus que la bande au total. Celle-là perd de l’information, d’une quantité bornée, et le dit dans son nom.

### 9.2 Expand

`Expand(start, end, step)` reconstruit une grille régulière en tenant chaque valeur jusqu’au changement suivant. Avant le premier point connu, il rend NaV : la série ne dit rien de ce que faisait le signal à ce moment-là, et un trou vaut mieux qu’une valeur inventée.

### 9.3 MarkSilences

Réduire une série brute comporte un piège : si l’instrument cesse d’émettre pendant une journée, rien dans la série réduite n’enregistre le silence — la dernière valeur semble simplement se maintenir.

`MarkSilences(maxGap)` insère un NaV partout où deux relevés sont espacés de plus de `maxGap`, daté `maxGap` après le dernier relevé précédant le silence — le moment où une alarme surveillant le capteur se serait déclenchée. Jusque-là, la valeur tenue est légitime : un capteur qui émet toutes les dix minutes n’est pas perdu à la onzième. Le silence survit alors à la réduction, sans passer par une grille.

## 10 Interpolation

Seule opération qui invente des données : ce qui en sort est une lecture de ce qui s’est probablement passé, pas un relevé.

### 10.1 Sur le temps, pas sur les rangs

Un trou est comblé d’après sa place **dans le temps**. Sur une série régularisée, ça ne change rien ; sur une série brute, ça change tout :

    10 mesuré à 00:00, un trou à 00:01, 20 mesuré à 01:00

       par le temps (cette bibliothèque) : 10,17
       par les rangs (l'ancienne)        : 15

Le trou est à une minute de son voisin de gauche et à cinquante-neuf de celui de droite. Quinze n’est pas une réponse défendable.

### 10.2 Sept méthodes

<a id="tab:interpolation"></a>

| Méthode | Comble avec | Pour |
|:---|:---|:---|
| `InterpLinear` | Une droite dans le temps | Une grandeur qui varie continûment |
| `InterpNearest` | Le voisin le plus proche | Un signal en marches ; garde une valeur mesurée |
| `InterpForwardFill` | La dernière valeur connue | Une consigne, un état, un compteur |
| `InterpBackwardFill` | La valeur connue suivante | Le début d’une série |
| `InterpLogLinear` | Un taux de croissance constant | Une grandeur qui se compose ; les deux voisins doivent être positifs |
| `InterpCubicSpline` | Une spline cubique naturelle | La régularité avant tout |
| `InterpMonotoneSpline` | Une spline PCHIP [14] | La régularité sans dépassement |

**Table 10.1** — Méthodes d’interpolation

**Pourquoi la spline monotone existe.** À travers une marche de 0 à 10, la spline naturelle comble le trou suivant avec **14,44** — une valeur supérieure à tous les relevés de la série, qu’aucun instrument n’a vue. La spline monotone reste à 10. Sur des données mesurées, c’est ce qui en fait la plus sûre des deux.

### 10.3 Seulement les trous, et seulement les courts

Les valeurs cassées sont laissées telles quelles (§2.5), et rien n’extrapole : un trou sans relevé d’un côté reste un trou, à l’exception des deux reports, qui s’appuient sur un seul voisin par construction.

Aucune méthode ne sait quelle durée de panne peut être franchie. Une demi-heure de température manquante se traverse ; trois jours, non — et un graphique qui cache la panne derrière une courbe lisse ment sans en avoir l’air :

    filled, err := ts.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)

La limite se mesure entre les relevés qui encadrent le trou : elle signifie donc la même chose sur une série brute et sur une grille.

## 11 Conteneurs

Un `TsContainer` garde les variantes d’un même signal : `raw`, `cleaned`, `hourly`, tout ce qu’une recette a produit.

Il les conserve **dans l’ordre où elles ont été déposées**, ce qu’une map Go ne fait pas : son parcours est aléatoire. La version précédente imprimait donc les variantes dans un ordre différent à chaque exécution, et une légende de graphique se remélangeait entre deux appels. Remplacer une variante lui conserve son rang.

`Get` distingue un nom jamais déposé d’un nom déposé à `nil` : le premier n’a jamais été demandé, le second n’a pas pu être produit, et l’affichage dit lequel.

## 12 Sorties

### 12.1 Terminal

    ts.PrettyPrint()       // le tableau des points
    ts.PrintStats()        // le résumé, en quatre sections
    ts.PrettyPrint(90, 95) // une fenêtre, pour une longue série

Les sentinelles s’affichent par leur nom — `NaV`, `NaN`, `NaDuration`, un tiret pour un instant inconnu. Un trou affiché « NaN », ou sous la forme d’une date de l’an 1, est un trou que personne ne remarque.

Les variantes `Fprint*` prennent un `io.Writer`, pour un journal ou un test.

### 12.2 JSON

`ToJSON` produit la forme que lit un frontend : des colonnes plutôt que des objets.

<a id="tab:json"></a>

| Clé | Contenu |
|:---|:---|
| `chron` | Les instants, en RFC 3339 |
| `meas`, `dmeas` | Mesures et variations, les non-nombres en `null` |
| `dchron_ns` | Les intervalles en nanosecondes, `NaDuration` en `null` |
| `stats` | Le résumé, mêmes noms de champs que `BasicStats`, en minuscules |

**Table 12.1** — Champs de l’export JSON

Tout non-nombre devient `null`, JSON n’ayant pas de NaN. **La distinction entre un trou et une erreur ne survit donc pas à la conversion** — ce sont les compteurs `nbreOfNaV` et `nbreOfNaN` du résumé qui la transportent.

## 13 Performances, mesurées

Toutes les durées citées dans ce guide — la table [13.1](#tab:performances) ci-dessous, mais aussi les tables [2.3](#tab:extraction) et [4.1](#tab:chargement) — ont été relevées sur la même machine : un Apple Mac mini M4 Pro, douze cœurs (huit de performance, quatre d’efficacité), 48 Gio de mémoire.

Les valeurs absolues n’ont donc pas de portée générale. Elles dépendent de la machine, de la version du compilateur, et de ce que celle-ci faisait par ailleurs. Ce qui porte le raisonnement, ce sont les rapports entre elles : cinquante contre un entre une médiane et une moyenne, un ordre de grandeur entre `Add` et `AddBatchData`.

<a id="tab:performances"></a>

| Opération                     | Coût                                    |
|:------------------------------|:----------------------------------------|
| Un point en mémoire           | 32 octets                               |
| Moyenne, min, max, somme      | ~1,2 ns par point, sans allocation      |
| Écart-type                    | ~2 ns par point, sans allocation        |
| Médiane, percentile           | ~50 ns par point — ils trient une copie |
| Un `Stats()` complet          | ~60 ns par point                        |
| `AddBatchData`, dans l’ordre  | ~0,03 µs par point                      |
| `AddBatchData`, en désordre   | ~0,5 µs par point                       |
| `Add`, un par un, en désordre | quadratique — à éviter                  |

**Table 13.1** — Coûts mesurés, par opération

La croissance est linéaire jusqu’à dix millions de points pour tout ce qui n’est pas médiane, celles-ci portant leur `n log n`.

## 14 Migration depuis la version précédente

La bibliothèque a été réécrite pour la `v0.2`. Ce qui a changé, et pourquoi :

<a id="tab:migration"></a>

| Avant | Maintenant | Pourquoi |
|:---|:---|:---|
| Un point rejeté devenait `NaN` | Il devient `NaV` | Une valeur aberrante ne détruit plus un mois de statistiques |
| Statistiques stockées dans la série | `Stats()` calcule à la demande | Des statistiques stockées se périmaient en silence |
| Un drapeau `deltasValid` | Un invariant | Rendre le mauvais état impossible vaut mieux que le signaler |
| `DataSeries` public | Privé, avec des accesseurs | Ce drapeau n’existait que parce que le tableau était public |
| Tolérance acceptée, jamais appliquée | Appliquée | Elle était silencieusement ignorée |
| Interpolation par rangs | Par le temps | Le rang est faux sur toute série irrégulière |
| Interpolation modifiant en place | Rend une nouvelle série | Cohérent avec le nettoyage et la régularisation |
| `MemId uint64` | `ID string` | Il n’était jamais rempli ; une chaîne accepte une clé de base ou un UUID |
| `Chstd` | Supprimé | La dispersion des horodatages ne dit rien ; `DChstd` dit l’utile |
| — | `ChFirstUsable` | Là où la fenêtre s’ouvre n’est pas là où les données commencent |
| — | `NbreOfNaV` | Distinguer les trous des erreurs |
| — | `InterpolateWithin`, `MarkSilences`, `ReduceWithDeadband` | Nouveaux |

**Table 14.1** — Changements apportés par la v0.2

Pour le contrat JSON : `id` est désormais une chaîne, `chstd` disparaît, et `nbreOfNaV`, `chFirstUsable` et `valAtFirstUsable` s’ajoutent.

## 15 Traçabilité des décisions

Chaque décision ci-dessus est soutenue par un test qui l’énonce en toutes lettres et par un message de commit qui consigne pourquoi elle a été prise. Quand un comportement surprend, `git log` et `git blame` sur le fichier l’expliquent mieux que le code.

## A Exemples d’utilisation

Les exemples qui suivent ne sont pas des extraits recopiés. Ce sont des fonctions `Example` des deux paquets : `go test` les compile, les exécute, et compare ce qu’elles écrivent au commentaire `// Output` qui les termine. Un exemple qui ne correspond plus au code fait échouer les tests.

C’est la seule forme de documentation qui ne puisse pas vieillir sans qu’on le sache, et la raison pour laquelle ce qui suit est extrait de `example_test.go` au moment de composer le guide plutôt que saisi une fois pour toutes. Les mêmes exemples se déplient sur pkg.go.dev, sous la fonction qu’ils illustrent.

Ils sont écrits dans le paquet de test externe : ils montrent donc l’import et le préfixe qu’un appelant écrit réellement, et non les raccourcis que seul le paquet lui-même s’autorise. Les commentaires sont en anglais, comme le code.

Deux fonctions auxiliaires, `at` et `show`, abrègent les exemples de `timeseries` : la première fabrique un relevé pris `n` minutes après 10:00 UTC, la seconde affiche une série à raison d’une ligne par relevé.

### A.1 `notavalue`

#### A.1.1 Les trois règles sur un même calcul

Un relevé manquant est ignoré, un calcul rompu se propage, et un ensemble où il n’y a rien à calculer ne donne pas zéro mais rien. Chapitre 2.

    func Example() {
        fmt.Println(nav.Format(nav.Mean([]float64{1, 2, nav.NaV, 3})))
        fmt.Println(nav.Format(nav.Mean([]float64{1, 2, math.NaN(), 3})))
        fmt.Println(nav.Format(nav.Mean([]float64{nav.NaV, nav.NaV})))
        fmt.Println(nav.Format(nav.Mean(nil)))
        // Output:
        // 2
        // NaN
        // NaV
        // NaV
    }

#### A.1.2 L’asymétrie de l’addition

L’addition traite le manque comme neutre, les trois autres opérations non. Une pluie mensuelle dont un jour n’a pas été relevé reste la pluie tombée ; une concentration manquante multipliée par un débit connu ne donne rien.

    func ExampleAdd() {
        fmt.Println(nav.Format(nav.Add(nav.NaV, 5)))
        fmt.Println(nav.Format(nav.Add(nav.NaV, nav.NaV)))
        fmt.Println(nav.Format(nav.Sub(nav.NaV, 5)))
        fmt.Println(nav.Format(nav.Mul(nav.NaV, 0)))
        fmt.Println(nav.Format(nav.Div(nav.NaV, 0)))
        fmt.Println(nav.Format(nav.Add(math.NaN(), nav.NaV)))
        // Output:
        // 5
        // NaV
        // NaV
        // NaV
        // NaV
        // NaN
    }

#### A.1.3 Choisir la première source disponible

Un capteur principal, son secours, puis une valeur modélisée. `Coalesce` passe sur ce qui manque, mais pas sur une erreur : un calcul rompu en amont ne se répare pas en passant au plan B.

    func ExampleCoalesce() {
        primary, backup, modeled := nav.NaV, nav.NaV, 18.2
        fmt.Println(nav.Format(nav.Coalesce(primary, backup, modeled)))

        broken := math.NaN()
        fmt.Println(nav.Format(nav.Coalesce(broken, modeled)))
        // Output:
        // 18.2
        // NaN
    }

#### A.1.4 Ce que vaut un résultat

Une moyenne ne dit pas sur combien de relevés elle repose. Les compteurs le disent, et distinguent les relevés absents des calculs rompus — ces derniers sont à chercher en amont.

    func ExampleCountUsable() {
        month := make([]float64, 31)
        for i := range month {
            month[i] = nav.NaV
        }
        month[0], month[1], month[2] = 12, 14, 13
        month[30] = math.NaN() // a computation went wrong on the last day

        fmt.Println("usable:   ", nav.CountUsable(month))
        fmt.Println("missing:  ", nav.CountNaV(month))
        fmt.Println("errors:   ", nav.CountNaN(month))
        fmt.Println("mean:     ", nav.Format(nav.Mean(month)))
        // Output:
        // usable:    3
        // missing:   27
        // errors:    1
        // mean:      NaN
    }

#### A.1.5 Garder la distinction à l’écran

Les verbes de la bibliothèque standard voient les deux sentinelles comme des NaN et écrivent « NaN » pour l’une comme pour l’autre. Toute la distinction disparaît dans une ligne de journal ou un message d’échec de test ; `Format` la conserve. Chapitre 12.

    func ExampleFormat() {
        fmt.Printf("%v %v\n", nav.NaV, math.NaN())
        fmt.Println(nav.Format(nav.NaV), nav.Format(math.NaN()))
        // Output:
        // NaN NaN
        // NaV NaN
    }

#### A.1.6 Une médiane qui n’invente pas de valeur

Sur un nombre pair de relevés, cette médiane rend la plus petite des deux valeurs centrales plutôt que leur moyenne. Sur une vanne codée 0 et 1, la définition classique renverrait 0,5 — un état que l’équipement n’a jamais occupé.

    func ExampleMedian() {
        fmt.Println(nav.Format(nav.Median([]float64{0, 0, 1, 1})))
        xs := []float64{10, 20, nav.NaV, 30, 40}
        fmt.Println(nav.Format(nav.Median(xs)))
        // Output:
        // 0
        // 20
    }

### A.2 `timeseries`

#### A.2.1 Le parcours complet

Des relevés bruts à un résumé : les valeurs invraisemblables écartées, une grille régulière, les trous courts comblés. Les relevés sont remis dans n’importe quel ordre, ce qui ne coûte rien — chapitre 4.

    func Example() {
        ts := timeseries.NewTimeSeries("outdoor temperature")
        ts.AddBatchData([]timeseries.Datum{
            at(40, 12.6),
            at(0, 11.8),
            at(20, 999), // the sensor went to its rail
            at(60, 13.1),
            at(120, 14.0), // one hour reported nothing at all
        })

        cleaned, rejected, err := ts.RemoveOutbounds(-40, 60)
        if err != nil {
            panic(err)
        }
        // The cleaned series still has five points: the rejected instant is
        // kept, its measurement now missing.
        fmt.Println("points:", cleaned.Len(), "rejected:", rejected.Len())

        hourly, err := cleaned.Regularize(time.Hour, timeseries.AggMean)
        if err != nil {
            panic(err)
        }
        fmt.Println("hourly grid:")
        show(hourly)

        filled, err := hourly.InterpolateWithin(
            timeseries.InterpLinear, 2*time.Hour)
        if err != nil {
            panic(err)
        }
        fmt.Println("after filling:")
        show(filled)

        fmt.Println("mean:", nav.Format(filled.Stats().Msmean))

        // Output:
        // points: 5 rejected: 1
        // hourly grid:
        //   10:00:00  11.8
        //   11:00:00  12.85
        //   12:00:00  14
        // after filling:
        //   10:00:00  11.8
        //   11:00:00  12.85
        //   12:00:00  14
        // mean: 12.883333333333333
    }

#### A.2.2 Un trou et une erreur ne sont pas le même accident

La première série a un relevé manquant, et garde une moyenne. La seconde porte le résultat d’un calcul rompu, et n’en a plus. La même asymétrie se lit sur la variation d’un point au suivant : un relevé de 20 après un trou n’est pas une hausse de 20. Chapitre 5.

    func Example_gapVersusError() {
        withGap := timeseries.NewTimeSeries("a day short")
        withGap.AddBatchData([]timeseries.Datum{
            at(0, 10), at(20, nav.NaV), at(40, 20),
        })

        withError := timeseries.NewTimeSeries("a broken computation")
        withError.AddBatchData([]timeseries.Datum{
            at(0, 10), at(20, math.NaN()), at(40, 20),
        })

        gap, bad := withGap.Stats(), withError.Stats()
        fmt.Println("gap:   ", nav.Format(gap.Msmean), "NaV:", gap.NbreOfNaV)
        fmt.Println("broken:", nav.Format(bad.Msmean), "NaV:", bad.NbreOfNaV)

        // The same asymmetry on the variation from one point to the next: a
        // reading of 20 after a gap is not a rise of 20, so the delta is
        // missing rather than invented.
        after := withGap.At(2)
        fmt.Println("variation across the gap:", nav.Format(after.Dmeas))

        // Output:
        // gap:    15 NaV: 1
        // broken: NaN NaV: 0
        // variation across the gap: NaV
    }

#### A.2.3 Nettoyer, et garder ce qu’on a rejeté

Le nettoyage rend deux séries : celle qui est nettoyée et celle des relevés écartés, pour que la décision puisse être revue. Dans la série nettoyée, l’instant rejeté est toujours là — sa mesure est devenue manquante, elle n’a pas disparu. Chapitre 6.

    func ExampleTimeSeries_RemoveOutbounds() {
        ts := timeseries.NewTimeSeries("pH")
        ts.AddBatchData([]timeseries.Datum{
            at(0, 7.1), at(20, -5), at(40, 7.3), at(60, 21), at(80, 7.0),
        })

        cleaned, rejected, err := ts.RemoveOutbounds(0, 14)
        if err != nil {
            panic(err)
        }
        fmt.Println("cleaned:")
        show(cleaned)
        fmt.Println("rejected:")
        show(rejected)

        // Output:
        // cleaned:
        //   10:00:00  7.1
        //   10:20:00  NaV
        //   10:40:00  7.3
        //   11:00:00  NaV
        //   11:20:00  7
        // rejected:
        //   10:20:00  -5
        //   11:00:00  21
    }

#### A.2.4 Un enregistreur qui dérive

Un enregistreur censé rapporter à l’heure juste rapporte quelques secondes trop tard. Sans tolérance, chacun de ces relevés tombe dans la fenêtre suivante, ce qui laisse une fenêtre vide et met deux relevés dans la suivante. Chapitre 7.

    func ExampleTimeSeries_RegularizeWithTolerance() {
        base := time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
        ts := timeseries.NewTimeSeries("drifting logger")
        ts.AddBatchData([]timeseries.Datum{
            timeseries.NewDatum(base, 10),
            timeseries.NewDatum(base.Add(time.Hour+4*time.Second), 11),
            timeseries.NewDatum(base.Add(2*time.Hour+9*time.Second), 12),
        })

        strict, err := ts.Regularize(time.Hour, timeseries.AggMean)
        if err != nil {
            panic(err)
        }
        fmt.Println("without tolerance:")
        show(strict)

        tolerant, err := ts.RegularizeWithTolerance(
            time.Hour, time.Minute, timeseries.AggMean)
        if err != nil {
            panic(err)
        }
        fmt.Println("with a minute of grace:")
        show(tolerant)

        // Output:
        // without tolerance:
        //   10:00:00  10
        //   11:00:00  NaV
        //   12:00:00  11
        //   13:00:00  12
        // with a minute of grace:
        //   10:00:00  10
        //   11:00:00  11
        //   12:00:00  12
    }

#### A.2.5 Combler un silence court, pas une panne

La limite se mesure entre les deux relevés qui encadrent le trou, et non entre les pas de la grille : elle veut donc dire la même chose sur une série régulière et sur une série brute. Chapitre 10.

    func ExampleTimeSeries_InterpolateWithin() {
        ts := timeseries.NewTimeSeries("flow rate")
        ts.AddBatchData([]timeseries.Datum{
            at(0, 100),
            at(20, nav.NaV), // one point missing: 40 minutes to bridge
            at(40, 140),
            at(60, nav.NaV), // an outage: 80 minutes to bridge
            at(80, nav.NaV),
            at(100, nav.NaV),
            at(120, 60),
        })

        // Note what the limit is measured on: not the 20 minutes of the
        // grid, but the 40 minutes between the readings on either side of
        // the gap. A limit of 30 minutes here would fill nothing at all.
        filled, err := ts.InterpolateWithin(
            timeseries.InterpLinear, 45*time.Minute)
        if err != nil {
            panic(err)
        }
        show(filled)

        // Output:
        //   10:00:00  100
        //   10:20:00  120
        //   10:40:00  140
        //   11:00:00  NaV
        //   11:20:00  NaV
        //   11:40:00  NaV
        //   12:00:00  60
    }

#### A.2.6 Comprimer un signal d’état, puis le restituer

Un signal qui passe son temps à ne pas bouger. `Reduce` garde les instants où quelque chose s’est produit, `Expand` remet la grille. Chapitre 9.

    func ExampleTimeSeries_Reduce() {
        ts := timeseries.NewTimeSeries("valve")
        ts.AddBatchData([]timeseries.Datum{
            at(0, 0), at(20, 0), at(40, 1), at(60, 1),
            at(80, 1), at(100, 0), at(120, 0),
        })

        reduced := ts.Reduce()
        fmt.Println("reduced to", reduced.Len(), "points out of", ts.Len())
        show(reduced)

        first, _ := ts.First()
        last, _ := ts.Last()
        back, err := reduced.Expand(first.Chron, last.Chron, 20*time.Minute)
        if err != nil {
            panic(err)
        }
        fmt.Println("expanded again:", back.Len(), "points")
        show(back)

        // Output:
        // reduced to 4 points out of 7
        //   10:00:00  0
        //   10:40:00  1
        //   11:40:00  0
        //   12:00:00  0
        // expanded again: 7 points
        //   10:00:00  0
        //   10:20:00  0
        //   10:40:00  1
        //   11:00:00  1
        //   11:20:00  1
        //   11:40:00  0
        //   12:00:00  0
    }

#### A.2.7 Parcourir un conteneur sans allouer

Un conteneur tient les séries qui vont ensemble — les relevés bruts, la version nettoyée, chaque variante produite en chemin. `MeasTo` est le compagnon de la boucle intérieure : on lui confie une tranche de longueur nulle, il la remplit au lieu d’en allouer une neuve, et un passage sur cent séries n’alloue qu’une fois. Chapitre 11.

    func ExampleTsContainer() {
        raw := timeseries.NewTimeSeries("raw")
        raw.AddBatchData([]timeseries.Datum{
            at(0, 11.8), at(20, 999), at(40, 12.6),
        })
        cleaned, _, err := raw.RemoveOutbounds(-40, 60)
        if err != nil {
            panic(err)
        }

        tsc := timeseries.NewTsContainer("outdoor temperature")
        tsc.Put("raw", raw)
        tsc.Put("cleaned", cleaned)

        buf := make([]float64, 0, 1024)
        for _, name := range tsc.Names() {
            ts := tsc.Series(name)
            buf = ts.MeasTo(buf[:0])
            fmt.Printf("%-8s n=%d usable=%d mean=%s\n", name, ts.Len(),
                nav.CountUsable(buf), nav.Format(nav.Mean(buf)))
        }

        // Output:
        // raw      n=3 usable=3 mean=341.1333333333333
        // cleaned  n=3 usable=2 mean=12.2
    }

## B Référence des interfaces

Les listes qui suivent donnent, pour chaque paquet, ce qu’il exporte : constantes, variables, fonctions, types avec leurs champs et leurs méthodes. Elles sont extraites de `go doc -all` et ne peuvent donc décrire ni une fonction qui n’existe plus, ni oublier celle qui vient d’être ajoutée.

Ce sont des signatures, sans les commentaires qui les accompagnent. Le godoc complet est sur pkg.go.dev, pour [`notavalue`](https://pkg.go.dev/github.com/fflamingodev/notavalue) [15] et pour [`timeseries`](https://pkg.go.dev/usefulrisk.com/timeseries) [16], où il est navigable et à jour de la version publiée. Le reproduire ici doublerait ce document pour répéter en anglais ce que les chapitres précédents expliquent en français.

L’ordre est celui de `go doc` : alphabétique à l’intérieur de chaque rubrique, les constructeurs et les méthodes d’un type venant après sa déclaration.

### B.1 `notavalue`

    import nav "github.com/fflamingodev/notavalue"

#### B.1.1 Variables

    var ErrPercentileRange = errors.New("percentile rank must be in (0, 100]")
    var NaV = math.Float64frombits(navBits)

#### B.1.2 Fonctions

    func Add(a, b float64) float64
    func Bounds(xs []float64) (minV, maxV float64)
    func Coalesce(xs ...float64) float64
    func CountNaN(xs []float64) int
    func CountNaV(xs []float64) int
    func CountNonNaV(xs []float64) int
    func CountUsable(xs []float64) int
    func Div(a, b float64) float64
    func Format(x float64) string
    func IsNaV(x float64) bool
    func IsStdNaN(x float64) bool
    func Max(xs []float64) float64
    func Mean(xs []float64) float64
    func Median(xs []float64) float64
    func Min(xs []float64) float64
    func Mul(a, b float64) float64
    func Percentile(xs []float64, p float64) (float64, error)
    func StdDev(xs []float64) float64
    func Sub(a, b float64) float64
    func Sum(xs []float64) float64

### B.2 `timeseries`

    import "usefulrisk.com/timeseries"

#### B.2.1 Constantes

    const NaDuration = time.Duration(math.MinInt64)

#### B.2.2 Variables

    var ErrAlarmArg = errors.New("timeseries: impossible alarm argument")
    var ErrCleaningArg = errors.New(
            "timeseries: impossible cleaning argument")
    var ErrInterpolationArg = errors.New(
            "timeseries: impossible interpolation argument")
    var ErrRegularizeArg = errors.New(
            "timeseries: impossible regularization argument")

#### B.2.3 Fonctions

    func AggCountUsable(window []float64) float64
    func AggFirst(window []float64) float64
    func AggFirstUsable(window []float64) float64
    func AggLast(window []float64) float64
    func AggLastUsable(window []float64) float64
    func AggMax(window []float64) float64
    func AggMean(window []float64) float64
    func AggMedian(window []float64) float64
    func AggMin(window []float64) float64
    func AggSlope(window []float64) float64
    func AggSum(window []float64) float64
    func AggregatorNames() []string
    func InterpolationNames() []string
    func IsNaDuration(d time.Duration) bool
    func NewID() string

#### B.2.4 Types et méthodes

    type AggFunc func(window []float64) float64
    func AggIntegral(step time.Duration) AggFunc
    func Aggregator(name string) (AggFunc, error)

    type BasicStats struct {
        Len int
        Chmin      time.Time
        ValAtChmin float64
        Chmax      time.Time
        ValAtChmax float64
        Chmed  time.Time
        Chmean time.Time
        ChFirstUsable    time.Time
        ValAtFirstUsable float64
        Msmin     float64
        ChAtMsmin time.Time
        Msmax     float64
        ChAtMsmax time.Time
        Msmean    float64
        Msmed     float64
        Msstd     float64
        DChmin     time.Duration
        ChAtDChmin time.Time
        DChmax     time.Duration
        ChAtDchmax time.Time
        DChmean    float64
        DChmed     float64
        DChstd     float64
        DMsmin  float64
        DMsmax  float64
        DMsmed  float64
        DMsmean float64
        DMsstd  float64
        NbreOfNaN int
        NbreOfNaV int
    }
    func (bs BasicStats) Fprint(w io.Writer, title string)
    func (bs BasicStats) ToJSON() *BasicStatsJSON

    type BasicStatsJSON struct {
        Len        int         `json:"len"`
        Chmin      time.Time   `json:"chmin"`
        ValAtChmin JSONFloat64 `json:"valAtChmin"`
        Chmax      time.Time   `json:"chmax"`
        ValAtChmax JSONFloat64 `json:"valAtChmax"`
        Chmed            time.Time   `json:"chmed"`
        Chmean           time.Time   `json:"chmean"`
        ChFirstUsable    time.Time   `json:"chFirstUsable"`
        ValAtFirstUsable JSONFloat64 `json:"valAtFirstUsable"`
        Msmin     JSONFloat64 `json:"msmin"`
        ChAtMsmin time.Time   `json:"chAtMsmin"`
        Msmax     JSONFloat64 `json:"msmax"`
        ChAtMsmax time.Time   `json:"chAtMsmax"`
        Msmean    JSONFloat64 `json:"msmean"`
        Msmed     JSONFloat64 `json:"msmed"`
        Msstd     JSONFloat64 `json:"msstd"`
        DChminNS   JSONDurationNS `json:"dChmin_ns"`
        ChAtDChmin time.Time      `json:"chAtDChmin"`
        DChmaxNS   JSONDurationNS `json:"dChmax_ns"`
        ChAtDChmax time.Time      `json:"chAtDChmax"`
        DChmeanNS  JSONDurationNS `json:"dChmean_ns"`
        DChmedNS   JSONDurationNS `json:"dChmed_ns"`
        DChstdNS   JSONDurationNS `json:"dChstd_ns"`
        DMsmin  JSONFloat64 `json:"dMsmin"`
        DMsmax  JSONFloat64 `json:"dMsmax"`
        DMsmed  JSONFloat64 `json:"dMsmed"`
        DMsmean JSONFloat64 `json:"dMsmean"`
        DMsstd  JSONFloat64 `json:"dMsstd"`
        NbreOfNaN int `json:"nbreOfNaN"`
        NbreOfNaV int `json:"nbreOfNaV"`
    }

    type DataUnit struct {
        Datum
        Dchron time.Duration
        Dmeas  float64
    }
    func NewDataUnit(chron time.Time, meas float64) DataUnit
    func (du DataUnit) Fprint(w io.Writer)
    func (du DataUnit) IsFirst() bool
    func (du DataUnit) PrettyPrint()

    type Datum struct {
        Chron time.Time
        Meas  float64
    }
    func NewDatum(chron time.Time, meas float64) Datum
    func (d Datum) IsMissing() bool

    type InterpolationMethod int
    const (
        InterpNone InterpolationMethod = iota
        InterpLinear
        InterpNearest
        InterpForwardFill
        InterpBackwardFill
        InterpLogLinear
        InterpCubicSpline
        InterpMonotoneSpline
    )
    func Interpolation(name string) (InterpolationMethod, error)
    func (m InterpolationMethod) String() string

    type JSONDurationNS time.Duration
    func (d JSONDurationNS) MarshalJSON() ([]byte, error)

    type JSONFloat64 float64
    func (f JSONFloat64) MarshalJSON() ([]byte, error)

    type TimeSeries struct {
        ID string
        Name string
        Comment string
    }
    func NewTimeSeries(name string) *TimeSeries
    func (ts *TimeSeries) Add(d Datum)
    func (ts *TimeSeries) AddBatchData(data []Datum)
    func (ts *TimeSeries) At(i int) DataUnit
    func (ts *TimeSeries) DownscaleDaily(agg AggFunc) (*TimeSeries, error)
    func (ts *TimeSeries) DownscaleMonthly(agg AggFunc) (*TimeSeries, error)
    func (ts *TimeSeries) DownscaleWeekly(agg AggFunc) (*TimeSeries, error)
    func (ts *TimeSeries) DownscaleYearly(agg AggFunc) (*TimeSeries, error)
    func (ts *TimeSeries) Expand(start, end time.Time, step time.Duration) (
            *TimeSeries, error)
    func (ts *TimeSeries) First() (DataUnit, bool)
    func (ts *TimeSeries) Fprint(w io.Writer, what ...int)
    func (ts *TimeSeries) FprintStats(w io.Writer)
    func (ts *TimeSeries) Interpolate(method InterpolationMethod) (
            *TimeSeries, error)
    func (ts *TimeSeries) InterpolateWithin(method InterpolationMethod,
            maxGap time.Duration) (*TimeSeries, error)
    func (ts *TimeSeries) Last() (DataUnit, bool)
    func (ts *TimeSeries) Len() int
    func (ts *TimeSeries) MarkSilences(maxGap time.Duration) (*TimeSeries,
            error)
    func (ts *TimeSeries) Meas() []float64
    func (ts *TimeSeries) MeasTo(dst []float64) []float64
    func (ts *TimeSeries) PrettyPrint(what ...int)
    func (ts *TimeSeries) PrettyPrintAll(what ...int)
    func (ts *TimeSeries) PrintStats()
    func (ts *TimeSeries) Range(f func(i int, du DataUnit) bool)
    func (ts *TimeSeries) Reduce() *TimeSeries
    func (ts *TimeSeries) ReduceWithDeadband(band float64) (*TimeSeries,
            error)
    func (ts *TimeSeries) Regularize(freq time.Duration, agg AggFunc) (
            *TimeSeries, error)
    func (ts *TimeSeries) RegularizeWithTolerance(freq,
            tolerance time.Duration, agg AggFunc) (*TimeSeries, error)
    func (ts *TimeSeries) RemoveOutbounds(min, max float64) (cleaned,
            rejected *TimeSeries, err error)
    func (ts *TimeSeries) RemovePeirceOutliers() (cleaned,
            rejected *TimeSeries, err error)
    func (ts *TimeSeries) RemovePercentileOutliers(low, high float64) (
            cleaned, rejected *TimeSeries, err error)
    func (ts *TimeSeries) RemoveZScoreOutliers(level float64) (cleaned,
            rejected *TimeSeries, err error)
    func (ts *TimeSeries) Stats() BasicStats
    func (ts *TimeSeries) ToJSON() *TimeSeriesJSON

    type TimeSeriesJSON struct {
        ID      string           `json:"id"`
        Name    string           `json:"name"`
        Comment string           `json:"comment,omitempty"`
        Chron   []time.Time      `json:"chron"`
        Meas    []JSONFloat64    `json:"meas"`
        Dchron  []JSONDurationNS `json:"dchron_ns,omitempty"`
        Dmeas   []JSONFloat64    `json:"dmeas,omitempty"`
        Stats   *BasicStatsJSON  `json:"stats,omitempty"`
    }

    type TsContainer struct {
        Name string
        Comment string
    }
    func NewTsContainer(name string) *TsContainer
    func (tsc *TsContainer) Delete(name string)
    func (tsc *TsContainer) Fprint(w io.Writer, what ...int)
    func (tsc *TsContainer) FprintStats(w io.Writer)
    func (tsc *TsContainer) Get(name string) (*TimeSeries, bool)
    func (tsc *TsContainer) Len() int
    func (tsc *TsContainer) Names() []string
    func (tsc *TsContainer) PrettyPrint(what ...int)
    func (tsc *TsContainer) PrintStats()
    func (tsc *TsContainer) Put(name string, ts *TimeSeries)
    func (tsc *TsContainer) Range(f func(name string, ts *TimeSeries) bool)
    func (tsc *TsContainer) Series(name string) *TimeSeries
    func (tsc *TsContainer) ToJSON() *TsContainerJSON

    type TsContainerJSON struct {
        Name    string                     `json:"name"`
        Comment string                     `json:"comment,omitempty"`
        Series  map[string]*TimeSeriesJSON `json:"series"`
    }

## Bibliographie

1. *IEEE standard for floating-point arithmetic*. IEEE, 2019. doi: [10.1109/IEEESTD.2019.8766229](https://doi.org/10.1109/IEEESTD.2019.8766229).
2. The PostgreSQL Global Development Group, “Aggregate functions: PostgreSQL documentation.” Accessed: Sept. 28, 2026. [Online]. Available: <https://www.postgresql.org/docs/current/functions-aggregate.html>
3. The PostgreSQL Global Development Group, “Database page layout: PostgreSQL documentation, heap tuple layout.” Accessed: Sept. 28, 2026. [Online]. Available: <https://www.postgresql.org/docs/current/storage-page-layout.html>
4. SQLite Consortium, “Database file format: Record format.” Accessed: Sept. 28, 2026. [Online]. Available: <https://www.sqlite.org/fileformat2.html>
5. The Apache Software Foundation, “Arrow columnar format: Validity bitmaps.” Accessed: Sept. 28, 2026. [Online]. Available: <https://arrow.apache.org/docs/format/Columnar.html>
6. The Prometheus Authors, “Querying basics: Prometheus documentation, staleness.” Accessed: Sept. 28, 2026. [Online]. Available: <https://prometheus.io/docs/prometheus/latest/querying/basics/>
7. The Prometheus Authors, “Model/value/value.go: Marqueurs de péremption dans le code de prometheus.” Accessed: Sept. 28, 2026. [Online]. Available: <https://github.com/prometheus/prometheus/blob/main/model/value/value.go>
8. InfluxData, “Explore data with InfluxQL: InfluxDB documentation, la clause fill().” Accessed: Sept. 28, 2026. [Online]. Available: <https://docs.influxdata.com/influxdb/v1/query_language/explore-data/>
9. The Go Authors, “Package database/sql: Bibliothèque standard de go.” Accessed: Sept. 28, 2026. [Online]. Available: <https://pkg.go.dev/database/sql>
10. The Go Authors, “Package time: Bibliothèque standard de go.” Accessed: Sept. 25, 2026. [Online]. Available: <https://pkg.go.dev/time>
11. G. Klyne and C. Newman, “Date and time on the internet: timestamps.” RFC 3339, IETF, 2002. doi: [10.17487/RFC3339](https://doi.org/10.17487/RFC3339).
12. B. Peirce, “Criterion for the rejection of doubtful observations,” *The Astronomical Journal*, vol. 2, no. 45, pp. 161–163, 1852.
13. S. M. Ross, “Peirce’s criterion for the elimination of suspect experimental data,” *Journal of Engineering Technology*, vol. 20, no. 2, pp. 38–41, 2003.
14. F. N. Fritsch and R. E. Carlson, “Monotone piecewise cubic interpolation,” *SIAM Journal on Numerical Analysis*, vol. 17, no. 2, pp. 238–246, 1980, doi: [10.1137/0717021](https://doi.org/10.1137/0717021).
15. F. Flament, “Package notavalue: Documentation de référence.” Accessed: Sept. 28, 2026. [Online]. Available: <https://pkg.go.dev/github.com/fflamingodev/notavalue>
16. F. Flament, “Package timeseries: Documentation de référence.” Accessed: Sept. 28, 2026. [Online]. Available: <https://pkg.go.dev/usefulrisk.com/timeseries>
