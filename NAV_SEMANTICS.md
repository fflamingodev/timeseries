# Propagation, non-propagation et NaV — guide de fond

> **Version bilingue.** La version française est suivie de la version
> anglaise. Les exemples et les tables sont partagés et ne sont pas
> dupliqués ; seul le texte explicatif change de langue.

---

## 🇫🇷 Version française

### Le problème en une phrase

Un chiffre qu'on n'a jamais eu, ce n'est pas la même chose qu'un chiffre
qu'on a mal calculé. La bibliothèque `timeseries` vous offre deux
marqueurs différents — `NaV` et le `NaN` standard — pour distinguer ces
deux situations, et deux règles de manipulation différentes selon que
vous combinez deux valeurs (opération *élémentaire*) ou que vous résumez
une collection (opération *agrégative*). Cette distinction est le point
qui surprend au premier contact, et ce guide en explique le pourquoi, le
comment et les pièges.

### Deux types de "non-valeurs"

Il est utile de clarifier le vocabulaire avant toute autre chose :

| Marqueur        | Sens sémantique                                   | Origine typique                                     |
| --------------- | ------------------------------------------------- | --------------------------------------------------- |
| `NaV`           | "la donnée n'a jamais existé / n'a pas été reçue" | capteur offline, champ absent, bucket vide          |
| `math.NaN()`    | "le calcul a cassé"                               | `0/0`, `log(-1)`, `Inf - Inf`, erreur numérique     |
| `NaDate`        | "pas d'horodatage valide"                         | champ `time.Time` zéro, timestamp malformé          |
| `NaDuration`    | "pas de durée valide"                             | `SafeSub(a, NaDate)`                                |

Les deux premiers sont des `float64` NaN-class — c'est-à-dire que
`math.IsNaN(x)` retourne `true` pour chacun — mais ils portent des bits
différents dans leur mantisse. Tout le code Go existant qui teste contre
NaN continue de fonctionner ; en revanche, `IsNaV(x)` retourne `true`
seulement pour NaV, et `IsStdNaN(x)` retourne `true` seulement pour un
NaN sans tag NaV.

### Pourquoi pas simplement la valeur 0 ?

Le raccourci qui vient naturellement à l'esprit, et d'autant plus en
Go, est d'utiliser `0` comme synonyme de "pas de donnée". C'est le
réflexe qu'on retrouve dans beaucoup de tables de capteurs, de
gestionnaires de formulaires et de structures de transport. Il est
doublement piégeux.

**Piège n°1 : zéro est une vraie valeur.** Dans tous les domaines
métier, zéro a un sens mesurable. Le thermomètre affiche exactement
0 °C quand l'eau gèle ; le compteur d'un logement vide lit 0 kWh
consommés sur la journée ; un compte à découvert apuré montre 0 € de
solde ; un débitmètre pendant une coupure volontaire reporte 0 L/s.
Si vous encodez "absence de mesure" et "mesure valide de zéro" de la
même façon, vous fusionnez deux situations que le métier distingue —
et vos statistiques deviennent fausses. La moyenne des températures
d'une semaine où le capteur a été offline 10 % du temps n'a rien à
voir avec la moyenne d'une semaine où il a lu 0 °C pendant 10 % du
temps.

**Piège n°2 : Go initialise à zéro par défaut.** En Go, tout champ
`float64` non affecté explicitement vaut `0.0`. Un struct fraîchement
construit dont un champ n'a jamais été touché est rigoureusement
indistinguable d'un struct dont le même champ a été affecté à zéro.

```go
type Reading struct {
    TempC float64
    Humid float64
}

var r Reading                 // r.TempC == 0.0 — pas initialisé, ou mesure à 0 ?
json.Unmarshal(data, &r)      // si "tempC" manque dans le JSON,
                              // r.TempC vaut toujours 0.0 silencieusement
```

Ce piège se retrouve partout : JSON dont un champ est absent, ligne
SQL avec un `NULL` déréférencé sans test `.Valid`, champ `protobuf`
ajouté au bout d'un message que l'ancien client ne peuple pas, valeur
par défaut d'un `flag` non fourni en ligne de commande. Dans tous ces
cas, `0.0` sert de remplisseur silencieux — et c'est précisément ce
silence qui est dangereux.

**Les alternatives classiques.** On peut contourner le problème avec
un champ `Valid bool` à côté de chaque valeur (c'est ce que fait
`database/sql` avec `sql.NullFloat64`), ou avec un pointeur `*float64`
qui peut être nil. Les deux fonctionnent, mais chacun double la
surface mémoire ou force une indirection sur toute la chaîne, et
surtout chacun demande une vigilance permanente : oublier un test
`.Valid`, ou déréférencer un pointeur après un nil-check qu'on
a zappé, et on retombe exactement dans le piège n°1.

**NaV est le bon compromis pour les `float64`.** Un seul `float64`
porte à la fois la valeur et le fait qu'elle est absente. Pas de
doublage de mémoire, pas d'indirection, pas de test à oublier :
`math.IsNaN(x)` retourne `true` pour NaV (ce qui court-circuite toute
arithmétique accidentelle), et `IsNaV(x)` dit précisément qu'il
s'agit d'une absence par nature. Les lectures réelles de zéro restent
strictement `0.0` et ne trompent personne. L'invariant "zéro n'est
jamais synonyme d'absence" tient de bout en bout du pipeline, y
compris lorsque vos données traversent un `json.Unmarshal`, une ligne
SQL ou un champ protobuf qui vous auraient, sinon, livré `0.0` à la
place de la réalité.

### Deux types d'opérations, deux règles

Il y a exactement deux catégories d'opérations numériques dans la
bibliothèque, et chacune obéit à une règle différente. **La distinction
est structurelle, pas arbitraire** : elle découle directement du type
mathématique de l'opération.

#### Opérations élémentaires : propagation stricte

Une opération élémentaire prend **deux valeurs** en entrée et produit
**une valeur** en sortie. C'est `Add`, `Sub`, `Mul`, `Div`. La règle :

> **Si au moins un opérande est une non-valeur, le résultat est une
> non-valeur.**

```go
Add(2, 3)            // 5
Add(2, 0)            // 2
Add(NaV, 5)          // NaV        (pas 5 !)
Add(5, NaV)          // NaV
Add(NaV, NaV)        // NaV
Sub(NaV, 5)          // NaV        (pas -5 !)
Sub(5, NaV)          // NaV
Mul(NaV, 0)          // NaV        (pas 0 !)
Div(NaV, 2)          // NaV
```

**Pourquoi ?** Parce que `Add(NaV, 5)` demande la somme de "je ne sais
pas" et de `5`. La réponse honnête est "je ne sais pas". Toute autre
réponse invente une information. Si vous répondiez `5`, vous traiteriez
`NaV` comme un zéro, ce qui changerait les totaux. Si vous répondiez
`NaN`, vous perdriez l'information que l'absence est volontaire. La
seule réponse cohérente est `NaV`.

**Piège courant — la soustraction.** Un design antérieur renvoyait
`Sub(NaV, 5) = -5` (en interprétant NaV comme zéro à gauche). C'est
faux : si je ne sais pas combien j'avais, je ne sais pas non plus
combien je perds en enlevant `5`. Le nouveau contrat renvoie `NaV`, ce
qui correspond à l'intuition des statisticiens et à ce que font
`pandas`, `R`, `NumPy` et `SQL`.

**Piège courant — NaV vs NaN en propagation.** Si un opérande est NaV
et l'autre un NaN standard, le résultat est NaV (le NaV "gagne").
C'est un choix délibéré : un NaV informatif vaut mieux qu'un NaN
anonyme. La seule exception est si les deux opérandes sont non-NaV mais
au moins un est NaN standard : là, le résultat est NaN.

```go
Add(NaV, math.NaN())   // NaV     (la cause "absence" est conservée)
Add(5,   math.NaN())   // NaN     (aucun NaV dans l'entrée)
```

#### Opérations agrégatives : NaV ignorée par défaut

Une opération agrégative prend **un tableau de valeurs** en entrée et
produit **un scalaire** en sortie. C'est `Sum`, `Mean`, `Median`,
`StdDev`, `Min`, `Max`. La règle :

> **Les valeurs NaV et NaN sont silencieusement écartées. Le scalaire
> retourné est calculé à partir des seules valeurs finies. Si après
> filtrage il ne reste rien, on retourne NaV.**

```go
Mean([]float64{1, 2, 3})                   // 2
Mean([]float64{1, 2, NaV, 3})              // 2         (moyenne de {1,2,3})
Mean([]float64{NaV, NaV, NaV})             // NaV       (aucune valeur finie)
Mean([]float64{})                          // NaV       (entrée vide)

Sum([]float64{1, NaV, 3})                  // 4
Sum([]float64{1, math.NaN(), 3})           // 4         (NaN aussi ignoré)
```

**Pourquoi ?** Parce qu'un agrégat répond à la question "donne-moi un
résumé de ce que tu as". Si votre capteur a raté deux mesures sur
soixante, vous voulez la moyenne des cinquante-huit mesures valides, pas
un `NaV` qui vous fait perdre toute l'heure. C'est ce que fait
`pandas.mean()` avec son défaut `skipna=True`, `numpy.nanmean`,
`postgres AVG(col)`, et à peu près toutes les conventions métier.

**Les variantes `*Strict`.** Si vous *voulez* que la présence d'une
seule valeur manquante fasse refuser l'agrégat entier, utilisez les
variantes nommées :

```go
SumStrict([]float64{1, NaV, 3})   // NaV (un seul NaV suffit)
MeanStrict([]float64{1, NaV, 3})  // NaV
```

Les deux politiques coexistent explicitement. Le nom force la
déclaration d'intention — vous ne glissez pas dans l'une par inadvertance.

### Le modèle mental en trois lignes

Pour décider de la règle qui s'applique, posez-vous trois questions :

1. **Combien de valeurs sont consommées pour produire UN résultat ?**
   Si c'est **deux** (les opérandes d'une opération arithmétique),
   propagation stricte. Si c'est **plusieurs** (toute la fenêtre d'une
   agrégation), skip par défaut.

2. **Est-ce que chaque entrée est indispensable à la sortie ?** Pour
   une addition oui, chaque opérande est indispensable et son absence
   rend le résultat non-défini. Pour une moyenne, l'absence d'un point
   parmi cinquante ne rend pas la moyenne non-définie, juste un peu
   moins précise.

3. **Quelle réponse est utile en pratique ?** Un `NaV` qui se propage
   partout dans un pipeline par un seul trou de capteur est
   généralement inutile ; un `NaV` qui se propage dans un `Dmeas` mal
   défini est, lui, exactement ce qu'il faut.

Si la question 3 vous laisse dans le doute, demandez-vous **qui écrit
le code de sortie** : l'outil qui affiche une moyenne veut un nombre
(skip), l'outil qui affiche un écart par rapport au point précédent
veut un trou (propagation).

### Exemples filés sur un cas réel

Supposons un capteur qui envoie une mesure toutes les minutes pendant
une heure. À `t=5` il est offline. À `t=23` le convertisseur
analogique-numérique renvoie une valeur corrompue qu'on encode en
`math.NaN()` pour la signaler comme un défaut numérique (à distinguer
d'une panne réseau).

La série ingérée ressemble donc à :

```
[20.1, 20.3, 20.2, 20.5, 20.4, NaV, 20.6, ..., math.NaN(), ..., 21.0]
```

#### Ce que vous obtenez pour différentes opérations

| Opération demandée                       | Résultat                                   | Règle appliquée                                                                    |
| ---------------------------------------- | ------------------------------------------ | ---------------------------------------------------------------------------------- |
| `ts.Msmean` (moyenne des mesures)         | moyenne des 58 valeurs valides             | agrégat, skip NaV + NaN                                                            |
| `ts.NbreOfNaV`                           | `1`                                        | comptage, distingue NaV                                                            |
| `ts.NbreOfNaN`                           | `2` (NaV est NaN-class, donc compté aussi) | comptage, toutes les non-valeurs                                                   |
| `DataSeries[5].Dmeas` (delta au t=5)      | `NaV` (`Sub(NaV, 20.4) = NaV`)              | élémentaire, propagation                                                            |
| `DataSeries[6].Dmeas` (delta au t=6)      | `NaV` (`Sub(20.6, NaV) = NaV`)              | élémentaire, propagation                                                            |
| `DataSeries[23].Dmeas` (delta au t=23)    | `NaN` (pas de NaV impliqué)                 | élémentaire, propagation, NaV n'intervient pas                                      |
| `ts.DMsmean` (moyenne des deltas)         | moyenne des deltas non-NaN finis            | agrégat sur les Dmeas, skip                                                         |
| `Interpolate(InterpLinear)`              | remplit *tous* les trous NaN-class          | choix de design : l'interpolation ne distingue pas NaV de NaN (les deux sont "à combler") |

La clé à retenir : **la propagation dans `Dmeas` est une fonctionnalité,
pas un bug**. Elle permet à la moyenne des deltas de rester honnête : on
n'ajoute pas un `20.6 − NaV` inventé dans la somme. Inversement, la
moyenne des **mesures** reste sur les 58 vraies lectures et ne se laisse
pas saboter par le trou.

### Pourquoi pas "toujours propager" ou "toujours ignorer" ?

Beaucoup d'utilisateurs, à leur première rencontre avec le sujet,
proposent une règle uniforme. Voyons pourquoi les deux extrêmes se
cassent la figure :

**"Tout propager".** Dans ce monde, une seule lecture manquante sur
votre série d'un mois rend `Mean` NaV, `Min` NaV, `Max` NaV, `Median`
NaV. Vous perdez les statistiques de tout un mois à cause d'une panne
de trois minutes. Vous êtes obligés de pré-nettoyer chaque série avant
tout calcul, à la main. Ce n'est pas un outil, c'est une punition.

**"Tout ignorer".** Dans ce monde, `Add(NaV, 5) = 5`, `Sub(NaV, b) = −b`,
`Mul(NaV, x) = x`. Tous vos calculs d'écarts deviennent des inventions.
Dans la série ci-dessus, le `Dmeas[5]` vaudrait `−20.4` (comme si la
mesure au `t=5` était `0`), et votre agrégat sur les deltas
surestimerait chaque transition. Plus grave : vous ne savez plus si un
delta est réel ou inventé, parce que rien n'est marqué différemment.

Le compromis "propagation en élémentaire, skip en agrégat" a le mérite
de coller à l'usage réel : on **préserve** l'information d'absence là
où elle est locale et significative (les deltas), on la **laisse
s'absorber** là où elle serait contre-productive (les résumés). C'est
exactement le choix que font `pandas`, `NumPy` (via `nanmean` et ses
amis), `R` (via `na.rm=TRUE`) et `SQL` (via les règles de `NULL` dans
les agrégats).

### Comparaison avec les autres conventions

| Système            | Élémentaire                | Agrégat                           | Remarque                             |
| ------------------ | -------------------------- | --------------------------------- | ------------------------------------ |
| **timeseries**     | propagation stricte         | skip par défaut, `*Strict` dispo  | c'est ce que ce guide documente      |
| **pandas**         | propagation                 | `skipna=True` par défaut          | même convention, nom différent       |
| **NumPy**          | propagation                 | `.mean()` propage, `nanmean()` skip | choix explicite par fonction         |
| **R**              | propagation                 | `na.rm=FALSE` par défaut (propage)| l'inverse de pandas pour les agrégats |
| **SQL**            | `NULL + 5 = NULL`           | `SUM(col)` ignore les NULL         | même convention que pandas           |

Si votre intuition vient de `R`, attention : les agrégats de
`timeseries` sautent les NaV par défaut, alors que R propage par défaut.
Si votre intuition vient de `pandas` ou `SQL`, tout va bien.

### FAQ — les questions qui reviennent

**Q : Pourquoi `Mean([1, 2, NaV, 3])` ne renvoie pas NaV ?**
Parce que `Mean` est un agrégat : il répond à "quelle est la moyenne
des valeurs valides que tu as ?". La réponse est `2`. Si vous vouliez
la sémantique stricte, utilisez `MeanStrict`.

**Q : Pourquoi `Add(NaV, 5)` renvoie NaV et pas 5 ?**
Parce que `Add` est élémentaire : il répond à "quelle est la somme des
deux nombres ?". Si un des deux est manquant, la somme est inconnue.
Remplacer par `5` inventerait que l'absence vaut zéro.

**Q : Si je veux vraiment tout casser à la première valeur manquante,
comment faire ?**
Utilisez `MeanStrict`, `SumStrict`, ou plus généralement gardez une
étape amont qui refuse l'entrée : `if ts.NbreOfNaV > 0 { return errors.New("données manquantes") }`.

**Q : Je n'ai pas envie d'utiliser NaV, je veux juste les NaN
standards. Est-ce grave ?**
Non. Toute la bibliothèque fonctionne avec `math.NaN()` — NaV est
simplement une version enrichie. Vous perdez juste la capacité de
distinguer "absent" de "calcul cassé" dans vos sorties. Les agrégats
ignorent les deux indifféremment.

**Q : Pourquoi `Interpolate` remplit-il les NaN aussi, et pas
seulement les NaV ?**
Parce que le rôle de l'interpolation est de combler tous les trous
visibles à l'affichage. Les deux marqueurs pointent vers l'absence
d'une valeur exploitable à cet instant. Si vous voulez interpoler
uniquement les NaV (en laissant les NaN intacts comme signal d'erreur),
filtrez avant : `for i, du := range ts.DataSeries { if IsStdNaN(du.Meas) { ... } }`.

**Q : Dans une chaîne de calculs, comment je sais ce qui est NaV et ce
qui est NaN à la fin ?**
Utilisez `IsNaV(x)` pour NaV, `IsStdNaN(x)` pour NaN standard,
`math.IsNaN(x)` pour les deux. Dans les statistiques, `BasicStats`
expose `NbreOfNaV` et `NbreOfNaN` séparément.

### Résumé en quatre lignes

1. **Propagation** : quand on combine deux valeurs dont une est NaV,
   on sort NaV.
2. **Skip** : quand on agrège plusieurs valeurs, on ignore les NaV.
3. **`*Strict`** : les variantes qui font propager aussi les agrégats,
   quand on en a besoin.
4. **NaV vs NaN** : tous deux sont NaN-class, tous deux sont traités
   par les mêmes règles, mais ils racontent deux histoires distinctes
   (absence par nature vs calcul cassé) et les compteurs du
   `BasicStats` les distinguent.

---

## 🇬🇧 English version

### The problem in one sentence

A number you never received is not the same thing as a number you
miscomputed. The `timeseries` library gives you two distinct markers —
`NaV` and plain `NaN` — to tell these two situations apart, and two
different handling rules depending on whether you are combining two
values (*element-wise* operation) or summarizing a collection
(*aggregate* operation). That distinction is the one thing that
surprises newcomers, and this guide walks through the why, the how and
the pitfalls.

### Two kinds of "non-values"

A quick vocabulary reset before anything else:

| Marker          | Semantic meaning                                    | Typical origin                                              |
| --------------- | --------------------------------------------------- | ----------------------------------------------------------- |
| `NaV`           | "the data never existed / was not received"         | sensor offline, missing field, empty bucket                 |
| `math.NaN()`    | "the computation broke"                             | `0/0`, `log(-1)`, `Inf - Inf`, numerical accident           |
| `NaDate`        | "no valid timestamp"                                | zero-value `time.Time`, malformed timestamp                 |
| `NaDuration`    | "no valid duration"                                 | `SafeSub(a, NaDate)`                                        |

The first two are both NaN-class `float64` — `math.IsNaN(x)` returns
`true` for either — but they carry different bit patterns in their
mantissa. All existing Go code that tests against NaN keeps working;
however, `IsNaV(x)` returns `true` only for NaV, and `IsStdNaN(x)`
returns `true` only for a plain NaN without the NaV tag.

### Why not just use zero?

The shortcut that comes to mind naturally, and all the more so in Go,
is to use `0` as a stand-in for "no data". It is the reflex you see in
many sensor tables, form-submission handlers and transport structs. It
is doubly treacherous.

**Trap #1: zero is a real value.** In every business domain, zero
carries a measurable meaning. A thermometer reads exactly 0 °C when
water freezes; a smart meter in an empty home reports 0 kWh consumed
for the day; a cleared overdraft account shows a balance of 0 €; a
flow sensor during a deliberate shutdown reports 0 L/s. If you encode
"no measurement" and "a valid measurement of zero" the same way, you
collapse two situations the business treats as distinct — and your
statistics become wrong. The average temperature over a week where the
sensor was offline 10 % of the time has nothing to do with the average
over a week where it genuinely read 0 °C for 10 % of the time.

**Trap #2: Go zero-initializes by default.** In Go, every unassigned
`float64` field evaluates to `0.0`. A freshly built struct whose field
was never touched is rigorously indistinguishable from one whose same
field was explicitly set to zero.

```go
type Reading struct {
    TempC float64
    Humid float64
}

var r Reading                 // r.TempC == 0.0 — uninitialized, or genuine zero?
json.Unmarshal(data, &r)      // if the "tempC" key is missing in the JSON,
                              // r.TempC silently stays 0.0
```

You meet this trap everywhere: a JSON whose field is absent, a SQL
row whose `NULL` was dereferenced without checking `.Valid`, a
`protobuf` field tacked onto the end of a message that the old client
never populates, a missing command-line flag that falls back to its
zero value. In each case, `0.0` is a silent stand-in — and the
silence is what makes it dangerous.

**Classic workarounds.** You can paper over it with a `Valid bool`
alongside each value (that is what `database/sql` does with
`sql.NullFloat64`), or with a pointer `*float64` that can be nil.
Both work, but each doubles the memory footprint or forces an
indirection through the whole reading pipeline, and both demand
constant vigilance: forgetting a `.Valid` check, or dereferencing a
pointer after a nil-check you skipped, drops you right back into
trap #1.

**NaV is the right fit for `float64`.** A single `float64` carries
both the value and the fact that it is absent. No doubled memory, no
indirection, no test to forget: `math.IsNaN(x)` returns `true` for
NaV (which short-circuits any accidental arithmetic), and `IsNaV(x)`
says precisely that this is a by-design absence. Real zero readings
stay strictly `0.0` and fool nobody. The invariant "zero never stands
for absence" holds end-to-end across the pipeline, including when
your data rides through `json.Unmarshal`, a SQL row, or a protobuf
field that would otherwise have delivered `0.0` in place of reality.

### Two kinds of operations, two rules

There are exactly two categories of numeric operations in the library,
and each follows a different rule. **The distinction is structural, not
arbitrary**: it falls out directly from the mathematical shape of the
operation.

#### Element-wise operations: strict propagation

An element-wise operation takes **two values** in and produces **one
value** out. These are `Add`, `Sub`, `Mul`, `Div`. The rule:

> **If at least one operand is a non-value, the result is a non-value.**

```go
Add(2, 3)            // 5
Add(2, 0)            // 2
Add(NaV, 5)          // NaV        (not 5!)
Add(5, NaV)          // NaV
Add(NaV, NaV)        // NaV
Sub(NaV, 5)          // NaV        (not -5!)
Sub(5, NaV)          // NaV
Mul(NaV, 0)          // NaV        (not 0!)
Div(NaV, 2)          // NaV
```

**Why?** Because `Add(NaV, 5)` asks for the sum of "I don't know" and
`5`. The honest answer is "I don't know". Any other answer invents
information. If you returned `5`, you would be treating `NaV` as zero,
which would distort totals. If you returned `NaN`, you would lose the
information that the absence is meaningful. `NaV` is the only
consistent answer.

**Common pitfall — subtraction.** An earlier design returned
`Sub(NaV, 5) = -5` (treating NaV as zero on the left). That was wrong:
if I do not know what I had, I do not know what I lose by taking `5`
away. The current contract returns `NaV`, which matches the
statistician's intuition and what `pandas`, `R`, `NumPy`, and `SQL`
all do.

**Common pitfall — NaV vs NaN in propagation.** If one operand is NaV
and the other is a plain NaN, the result is NaV (NaV "wins"). This is
a deliberate choice: an informative NaV is worth more than an anonymous
NaN. The only exception is when both operands are non-NaV but at least
one is plain NaN — then the result is NaN.

```go
Add(NaV, math.NaN())   // NaV      (the "absence" cause is preserved)
Add(5,   math.NaN())   // NaN      (no NaV in the input)
```

#### Aggregate operations: NaV skipped by default

An aggregate operation takes **an array of values** in and produces
**one scalar** out. These are `Sum`, `Mean`, `Median`, `StdDev`, `Min`,
`Max`. The rule:

> **NaV and NaN values are silently skipped. The scalar returned is
> computed from the remaining finite values only. If after filtering
> nothing remains, NaV is returned.**

```go
Mean([]float64{1, 2, 3})                   // 2
Mean([]float64{1, 2, NaV, 3})              // 2         (average of {1,2,3})
Mean([]float64{NaV, NaV, NaV})             // NaV       (no finite value)
Mean([]float64{})                          // NaV       (empty input)

Sum([]float64{1, NaV, 3})                  // 4
Sum([]float64{1, math.NaN(), 3})           // 4         (NaN skipped too)
```

**Why?** Because an aggregate answers the question "give me a summary
of what you have". If your sensor missed two readings out of sixty,
you want the mean of the fifty-eight valid readings, not a `NaV` that
erases the entire hour. This is what `pandas.mean()` does by default
with `skipna=True`, what `numpy.nanmean` does, what `postgres AVG(col)`
does, and essentially every business convention ever published.

**The `*Strict` variants.** If you *do* want a single missing value to
make the whole aggregate refuse, use the explicitly-named variants:

```go
SumStrict([]float64{1, NaV, 3})   // NaV (a single NaV is enough)
MeanStrict([]float64{1, NaV, 3})  // NaV
```

Both policies coexist explicitly. The name forces you to declare your
intent — you cannot drift into one by accident.

### The mental model in three lines

To figure out which rule applies, ask yourself three questions:

1. **How many values are consumed to produce ONE result?** If **two**
   (the operands of an arithmetic operation), strict propagation. If
   **many** (a whole window of an aggregation), skip by default.

2. **Is every input indispensable to the output?** For an addition,
   yes — every operand is indispensable and its absence makes the
   result undefined. For a mean, a missing point in fifty does not
   make the mean undefined, just marginally less precise.

3. **Which answer is actually useful?** A `NaV` that propagates
   everywhere in a pipeline because of a single sensor blip is usually
   useless; a `NaV` that propagates into a `Dmeas` that was genuinely
   not computable is exactly what you want.

If question 3 leaves you in doubt, ask **who consumes the output**:
the tool that shows a mean wants a number (skip); the tool that shows
a delta from the previous point wants a hole (propagation).

### Worked examples on a realistic case

Suppose a sensor emits a measurement every minute for one hour. At
`t=5` it goes offline. At `t=23` the analog-to-digital converter
returns a corrupted value that you encode as `math.NaN()` to flag it
as a numerical defect (distinct from a network outage).

The ingested series therefore looks like:

```
[20.1, 20.3, 20.2, 20.5, 20.4, NaV, 20.6, ..., math.NaN(), ..., 21.0]
```

#### What you get for different operations

| Operation                              | Result                                      | Rule applied                                                                        |
| -------------------------------------- | ------------------------------------------- | ----------------------------------------------------------------------------------- |
| `ts.Msmean` (mean of Meas)              | mean of the 58 valid values                  | aggregate, skip NaV + NaN                                                            |
| `ts.NbreOfNaV`                         | `1`                                          | counter, NaV-specific                                                                |
| `ts.NbreOfNaN`                         | `2` (NaV is NaN-class, so counted too)       | counter, all non-values                                                              |
| `DataSeries[5].Dmeas` (delta at t=5)    | `NaV` (`Sub(NaV, 20.4) = NaV`)                | element-wise, propagation                                                            |
| `DataSeries[6].Dmeas` (delta at t=6)    | `NaV` (`Sub(20.6, NaV) = NaV`)                | element-wise, propagation                                                            |
| `DataSeries[23].Dmeas` (delta at t=23)  | `NaN` (no NaV involved)                       | element-wise, propagation, NaV does not enter                                        |
| `ts.DMsmean` (mean of the deltas)       | mean of the finite non-NaN deltas             | aggregate over Dmeas, skip                                                           |
| `Interpolate(InterpLinear)`            | fills *every* NaN-class hole                 | design choice: interpolation does not distinguish NaV from NaN (both are "to fill") |

The takeaway: **propagation into `Dmeas` is a feature, not a bug**. It
lets the mean of the deltas stay honest — you do not inject a fabricated
`20.6 − NaV` into the sum. Conversely, the mean of the **measurements**
stays on the 58 actual readings and is not derailed by the hole.

### Why not "always propagate" or "always skip"?

Many users, on their first encounter with the topic, propose a uniform
rule. Here is why both extremes fall apart:

**"Propagate everywhere".** In this world, a single missing reading in
your month-long series makes `Mean` NaV, `Min` NaV, `Max` NaV, `Median`
NaV. You lose a month of statistics because of a three-minute outage.
You are forced to pre-clean every series by hand before any computation.
That is not a tool, that is a punishment.

**"Skip everywhere".** In this world, `Add(NaV, 5) = 5`,
`Sub(NaV, b) = −b`, `Mul(NaV, x) = x`. Every one of your delta
calculations becomes a fabrication. In the series above, `Dmeas[5]`
would be `−20.4` (as if the `t=5` measurement were `0`), and your
aggregate over the deltas would overestimate every transition. Worse:
you can no longer tell a real delta from an invented one, because
nothing is marked differently.

The "propagate element-wise, skip aggregate" compromise matches real
usage: you **preserve** the absence information where it is local and
meaningful (the deltas), and you **let it absorb** where preserving it
would be counter-productive (the summaries). That is exactly what
`pandas`, `NumPy` (via `nanmean` and friends), `R` (via `na.rm=TRUE`),
and `SQL` (via the `NULL` rules in aggregate functions) all do.

### Comparison with other conventions

| System             | Element-wise               | Aggregate                              | Note                                   |
| ------------------ | -------------------------- | -------------------------------------- | -------------------------------------- |
| **timeseries**     | strict propagation          | skip by default, `*Strict` available    | what this guide documents              |
| **pandas**         | propagation                 | `skipna=True` default                   | same convention, different name        |
| **NumPy**          | propagation                 | `.mean()` propagates, `nanmean()` skips  | explicit choice per function           |
| **R**              | propagation                 | `na.rm=FALSE` default (propagates)      | opposite of pandas for aggregates      |
| **SQL**            | `NULL + 5 = NULL`           | `SUM(col)` ignores NULLs                | same convention as pandas              |

If your intuition comes from `R`, beware: the `timeseries` aggregates
skip NaV by default, while R propagates by default. If your intuition
comes from `pandas` or `SQL`, you are already in familiar territory.

### FAQ — the questions that come back

**Q: Why does `Mean([1, 2, NaV, 3])` not return NaV?**
Because `Mean` is an aggregate: it answers "what is the average of the
valid values you have?". The answer is `2`. If you wanted the strict
semantics, use `MeanStrict`.

**Q: Why does `Add(NaV, 5)` return NaV and not 5?**
Because `Add` is element-wise: it answers "what is the sum of these
two numbers?". If one of them is missing, the sum is unknown.
Substituting `5` would invent that "absent" means "zero".

**Q: If I really want everything to blow up on a single missing
value, how do I do that?**
Use `MeanStrict`, `SumStrict`, or more generally guard upstream: `if
ts.NbreOfNaV > 0 { return errors.New("missing data") }`.

**Q: I do not want to bother with NaV, I just use plain NaN. Is that
a problem?**
No. The entire library works with `math.NaN()` — NaV is just an
enriched version. You only lose the ability to distinguish "absent"
from "broken computation" in your outputs. Aggregates skip both
indifferently.

**Q: Why does `Interpolate` fill NaN too, not just NaV?**
Because the job of interpolation is to fill every visible hole at
display time. Both markers point to the lack of a usable value at that
instant. If you want to interpolate NaV only (keeping NaN intact as an
error signal), filter upstream:
`for i, du := range ts.DataSeries { if IsStdNaN(du.Meas) { ... } }`.

**Q: In a chain of computations, how do I tell what is NaV and what
is NaN at the end?**
Use `IsNaV(x)` for NaV, `IsStdNaN(x)` for plain NaN, `math.IsNaN(x)`
for both. In statistics, `BasicStats` exposes `NbreOfNaV` and
`NbreOfNaN` separately.

### Summary in four lines

1. **Propagation**: when combining two values and one is NaV, you
   return NaV.
2. **Skip**: when aggregating many values, you ignore NaVs.
3. **`*Strict`**: explicit variants that make aggregates propagate too,
   when you need it.
4. **NaV vs NaN**: both are NaN-class, both are treated by the same
   rules, but they tell two distinct stories (absent by nature vs
   broken computation) and the `BasicStats` counters keep them apart.

---

*Dernière révision / last revision: avril 2026 / April 2026.*
