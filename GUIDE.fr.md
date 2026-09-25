<!-- Fichier produit par « make guide-fr-md » depuis docs/guide-fr.tex.
     Les modifications faites ici seront perdues à la prochaine génération. -->

# Guide de la bibliothèque *timeseries*

**Traitement des séries temporelles à trous** — Frédéric Flament

*Ce guide est écrit en LaTeX ; la version composée, avec sa table des
matières et sa bibliographie, se construit par `make guide-fr`. La
version anglaise est dans [GUIDE.md](GUIDE.md).*

*`timeseries` traite des séries de relevés tels qu’ils arrivent : à des instants irréguliers, avec des trous là où l’instrument s’est tu, et des valeurs auxquelles personne ne croit. Elle les nettoie, les pose sur une grille régulière, comble ce qui peut l’être, les résume — sans jamais transformer un relevé manquant en zéro, ni laisser un relevé aberrant détruire un mois de statistiques.*

Elle traite aussi les signaux à faible contenu informatif — un état, une consigne, un compteur, qui répètent la même valeur des heures durant — en les ramenant à leurs changements, et sait les reconstituer ensuite.

Le tout à l’échelle du parc et non du capteur isolé : les agrégats parcourent leurs données une fois et n’allouent rien, le chargement d’un lot est en $`n\log n`$ là où l’insertion point par point serait quadratique, et le coût par point reste constant jusqu’à dix millions de relevés. Les mesures sont dans le guide, avec les variantes écartées.

## Introduction

`timeseries` est une bibliothèque Go qui traite des séries de relevés mesurés. Elle répond à quatre questions.

**Que faire d’un relevé qui n’existe pas ?** Un trou n’est ni un zéro, ni une erreur, ni une ligne à supprimer. La bibliothèque le représente par un **NaV**, un NaN porteur d’un repère, que les calculs ignorent au lieu de s’y arrêter — là où un NaN ordinaire, réservé aux erreurs de calcul, continue de se propager. Une moyenne mensuelle survit à un jour manquant ; elle ne survit pas à une division par zéro, et c’est la différence qu’il fallait pouvoir exprimer.

**Que faire d’un instrument qui n’est pas un métronome ?** Les relevés annoncés « toutes les heures » arrivent à 00:57, 02:03, 03:00. La régularisation les repose sur une grille de pas fixe, avec une tolérance pour les enregistreurs qui dérivent, et des trous là où aucun relevé n’est arrivé. Deux séries traitées au même pas deviennent alors comparables point à point, condition de tout calcul qui les met en relation.

**Que faire d’un signal qui ne dit presque rien ?** Une porte, une consigne, un état, un compteur : la plupart des relevés n’apportent aucune information, puisque la valeur n’a pas bougé. La bibliothèque les réduit à leurs changements — le premier relevé, le dernier, et ce qui s’est passé entre les deux — et sait les reconstituer à la demande. Une bande morte permet de n’enregistrer qu’au-delà d’un écart donné, avec une perte bornée et connue. Et comme un silence prolongé se confond, après réduction, avec une valeur qui se maintient, `MarkSilences` le marque comme un trou avant que l’information ne disparaisse.

**Que faire quand il y en a beaucoup ?** Un capteur ne pose pas de problème ; mille capteurs interrogés sur un an en posent. Les agrégats parcourent donc leurs données une seule fois et n’allouent rien, le chargement d’un lot trie une fois au lieu d’insérer point par point — une seconde contre deux minutes et demie pour un million de relevés — et un tampon réutilisable permet de boucler sur des centaines de séries sans qu’une allocation ait lieu. Chacun de ces choix est mesuré, et les variantes écartées sont conservées dans le dépôt avec leurs chiffres.

Le reste du guide détaille ces quatre réponses, les conventions de temps qui les sous-tendent, et ce que chaque opération coûte.

## À quoi ressemble vraiment une série mesurée

Une série temporelle, dans un manuel, c’est une valeur par instant sur une grille régulière. Une série mesurée ne l’est jamais.

L’enregistreur annonce « toutes les heures » et rend ses relevés à 00:57, 02:03, 03:00, 04:01. Le capteur tombe pour une nuit. Une jointure laisse une ligne manquante. Une pile meurt en milieu d’après-midi et les derniers relevés avant la panne n’ont plus de sens. Une division par zéro en amont transforme une valeur en déchet. Un technicien remet un compteur à zéro et la série saute de dix mille.

Chacun de ces cas est ordinaire, et chacun fausse silencieusement un résultat s’il est traité à la légère. Cette bibliothèque existe pour les traiter explicitement, et pour que ce traitement reste visible ensuite.

À l’autre extrémité du problème se trouvent les séries qui ne disent presque rien. Une porte reste fermée des heures durant, une consigne ne bouge pas de la semaine, un état vaut 0 ou 1 : relever un tel signal chaque minute produit des milliers de points dont deux ou trois seulement portent une information. Les traiter comme une série ordinaire coûte de la mémoire, du calcul et de la lisibilité pour rien. Le chapitre 11 leur est consacré.

## Le problème des données manquantes

Comment savoir qu’une donnée est manquante ? Ou plutôt, comment caractériser, dans les calculs, dans les rapports, et dans le déclenchement d’action, une donnée qui n’est pas là ? Une solution courante est de déclencher une action à partir d’une absence de donnée pendant une durée fixée à l’avance. Si on examine les solutions apportées dans les logiciels courants, nous avons opté pour une solution utilisant le *NaN-boxing*.

## NaV pour « Not a Value » : une stratégie pour les données manquantes

### Définition de NaV

La norme IEEE 754 (IEEE Standard for Floating-Point Arithmetic 2019), qui régit les nombres flottants, laisse libre le contenu de la mantisse d’un NaN : n’importe quelle valeur non nulle en fait un NaN valide. Nous utilisons cette liberté pour introduire un nombre particulier, le **NaV**, un NaN silencieux dont le bit 48 de la mantisse est allumé. Il reste un `float64` ordinaire : un `[``]``float64` n’a besoin d’aucun masque parallèle, et `math.IsNaN` le reconnaît toujours, si bien que le code déjà écrit pour se prémunir des NaN se prémunit aussi des NaV.

Dans tout ce guide, un **trou** désigne un relevé manquant, c’est-à-dire un NaV : un instant où la mesure aurait dû avoir lieu et n’a pas eu lieu.

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
&#10;</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Signification</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Exemple</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Traitement</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><strong>NaV</strong></td>
<td style="text-align: left;">Rien n’a été mesuré</td>
<td style="text-align: left;">Capteur hors service, ligne absente, point rejeté</td>
<td style="text-align: left;">Ignoré</td>
</tr>
<tr>
<td style="text-align: left;"><strong>NaN</strong></td>
<td style="text-align: left;">Un calcul a cassé</td>
<td style="text-align: left;"><code>0/0</code> en amont, <code>log(-1)</code></td>
<td style="text-align: left;">Se propage</td>
</tr>
</tbody>
</table>

### Ce qui distingue le NaV du NaN

- Le NaN a été créé pour les erreurs de calcul, une donnée manquante n’est pas une erreur de calcul

- Une donnée manquante ne doit pas nécessairement se propager dans tous les calculs subséquents, or les NaN propagent l’indication de l’erreur de calcul

### Comportement attendu d’un trou

1.  **Un trou n’arrête jamais un calcul.** Il est ignoré par les agrégats et neutre dans l’addition.

2.  **Une erreur se propage toujours.** Quand un trou et une erreur se rencontrent, l’erreur l’emporte.

3.  **Quand il ne reste rien à calculer, le résultat est un trou** — jamais zéro.

La troisième règle appelle un point d’attention. La moyenne d’une série vide vaut NaV, pas 0. Une moyenne nulle est une affirmation sur les données : elle dit que les relevés se compensent, ou, plus simplement, que la température en Celsius était au point de gel. Sur une série vide, il n’y a rien à affirmer, et répondre zéro serait une invention qu’aucun lecteur ultérieur ne peut détecter.

### Ce que les règles produisent

Les conséquences ne sont pas nécessairement triviales:

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Situation</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Résultat</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Pourquoi</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;">Moyenne de <span><code>[</code></span><code>1, 2, NaV, 3</code><span><code>]</code></span></td>
<td style="text-align: left;"><code>2</code></td>
<td style="text-align: left;">Le trou est ignoré ; le diviseur vaut 3, pas 4</td>
</tr>
<tr>
<td style="text-align: left;">Moyenne de <span><code>[</code></span><code>1, 2, NaN, 3</code><span><code>]</code></span></td>
<td style="text-align: left;"><code>NaN</code></td>
<td style="text-align: left;">Une erreur en amont doit rester visible</td>
</tr>
<tr>
<td style="text-align: left;">Moyenne de <span><code>[</code></span><code>NaV, NaV</code><span><code>]</code></span></td>
<td style="text-align: left;"><code>NaV</code></td>
<td style="text-align: left;">Rien à dire</td>
</tr>
<tr>
<td style="text-align: left;"><code>Sub(5, NaV)</code></td>
<td style="text-align: left;"><code>NaV</code></td>
<td style="text-align: left;">Une différence exige ses deux termes ; rendre 5 reviendrait à lire le trou comme un zéro</td>
</tr>
<tr>
<td style="text-align: left;"><code>Add(NaV, 5)</code></td>
<td style="text-align: left;"><code>5</code></td>
<td style="text-align: left;">Une somme tolère un terme absent</td>
</tr>
<tr>
<td style="text-align: left;"><code>Mul(NaV, 0)</code></td>
<td style="text-align: left;"><code>NaV</code></td>
<td style="text-align: left;">Une quantité inconnue de quelque chose reste inconnue</td>
</tr>
<tr>
<td style="text-align: left;">Variation après un trou</td>
<td style="text-align: left;"><code>NaV</code></td>
<td style="text-align: left;">Un relevé de 14 après un trou n’est pas une hausse de 14</td>
</tr>
<tr>
<td style="text-align: left;">Point rejeté comme aberrant</td>
<td style="text-align: left;"><code>NaV</code></td>
<td style="text-align: left;">Il a été mesuré mais n’est pas cru : absent, pas cassé</td>
</tr>
<tr>
<td style="text-align: left;">Interpoler un <code>NaN</code></td>
<td style="text-align: left;">laissé tel quel</td>
<td style="text-align: left;">Recouvrir une erreur d’un nombre plausible est la façon dont un bug cesse d’être remarqué</td>
</tr>
</tbody>
</table>

L’asymétrie entre `Add` et `Sub` peut provoquer un haussement de sourcils. La logique est la suivante: une somme accumule des termes indépendants, et un terme absent laisse les autres intacts — c’est ce qui permet à une moyenne mensuelle de survivre à un jour manquant. Une différence compare deux valeurs précises ; si l’une est inconnue, la différence l’est aussi, et toute autre réponse fabrique une variation que personne n’a mesurée.

### Le NaN-boxing en détail

Le NaN-boxing consiste à ranger de l’information **à l’intérieur** d’un nombre flottant, dans les bits que la norme IEEE 754 laisse libres. Pour comprendre où ils sont, il faut ouvrir un `float64`.

#### La structure d’un flottant

Un `float64` occupe 64 bits, répartis par la norme IEEE-754 en trois champs :

     ┌─┬───────────┬────────────────────────────────────────────────────┐
     │S│  exposant │                     mantisse                       │
     └─┴───────────┴────────────────────────────────────────────────────┘
      1     11 bits                      52 bits

La valeur ordinaire se lit « mantisse × 2^exposant », avec le signe devant. Mais la norme réserve une configuration : **quand les 11 bits d’exposant valent tous 1**, ce n’est plus un nombre.

- Si la mantisse vaut zéro, c’est l’infini, positif ou négatif selon le signe.

- Si la mantisse vaut autre chose que zéro, c’est un **NaN**.

La norme ne dit pas *quelle* valeur la mantisse doit prendre. N’importe laquelle, pourvu qu’elle ne soit pas nulle, fait un NaN parfaitement valide. Il y a 2⁵² − 1 mantisses possibles, deux signes, soit **environ neuf millions de milliards de configurations qui sont toutes des NaN** — dont la moitié, celles dites silencieuses, sont utilisables sans risque. Toutes indiscernables pour l’arithmétique, et toutes perdues si personne ne s’en sert.

#### Silencieux et signalant

La norme distingue encore deux familles, par le bit de poids fort de la mantisse :

- **NaN silencieux** (bit à 1) : il traverse les calculs sans bruit. C’est celui que rend `math.NaN()`, et celui que produit toute opération invalide sur un processeur courant.

- **NaN signalant** (bit à 0) : il est censé déclencher une exception matérielle. En pratique, Go ne l’utilise pas, et la plupart des environnements le convertissent en silencieux dès la première opération. On ne s’en sert pas ici.

Il reste donc **51 bits libres** dans la mantisse d’un NaN silencieux. C’est de la place perdue, que rien n’utilise.

#### Ce que la bibliothèque en fait

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

1.  **Aucun surcoût mémoire.** Un NaV est un `float64`. Un tableau de mesures reste un `[``]``float64`, sans tableau compagnon.

2.  **Le code existant continue de fonctionner.** `math.IsNaN(NaV)` est vrai, donc tout code déjà écrit pour se prémunir des NaN se prémunit aussi des NaV. Rien à recompiler, rien à auditer.

3.  **La distinction survit au transport binaire.** Copier, sérialiser en binaire, passer par `math.Float64bits` : le repère voyage avec la valeur, puisqu’il *est* la valeur.

4.  **Il reste 50 bits libres.** On pourrait un jour distinguer « jamais mesuré », « rejeté comme aberrant », « interpolé », sans rien casser de l’existant.

#### Ce que ça coûte

- **L’arithmétique doit passer par les fonctions du paquet.** Ce que devient le repère à travers un `-` ordinaire dépend du processeur (§4.7). C’est la contrainte principale.

- **La sérialisation texte perd le repère.** JSON n’a pas de NaN : tout non-nombre devient `null`, et la distinction ne subsiste que dans les compteurs (§14.2).

- **C’est une technique peu connue.** Un relecteur qui découvre la bibliothèque doit d’abord comprendre ce qu’il lit — ce chapitre existe pour ça.

### Pourquoi pas une structure avec un booléen

C’est la solution la plus répandue :

    type Valued struct {
        Value float64
        Valid bool
    }

C’est ce que font `sql.NullFloat64` en Go, `Option<f64>` en Rust, `double?` en C#, `Optional<Double>` en Java. Six raisons de ne pas l’avoir reprise.

#### 1. Elle double la mémoire

    float64                       :  8 octets
    struct{ float64; bool }       : 16 octets

Le booléen n’occupe qu’un octet, mais l’alignement en impose huit : sept octets sont perdus par point. Sur un million de mesures, **8 Mo deviennent 16 Mo** ; sur dix millions, 80 deviennent 160.

Un masque parallèle `[``]``bool` fait mieux — 9 Mo — mais au prix d’un second tableau à transporter, à découper et à trier en même temps que le premier, et qu’on oublie à la première refonte.

#### 2. Elle coupe la bibliothèque du reste du monde

Tout ce qui calcule sur des flottants, en Go, attend un `[``]``float64`: gonum, les transformées de Fourier, les bibliothèques de statistiques, et `notavalue` lui-même.

Avec une structure, chaque appel exige d’abord une extraction — une boucle et une allocation proportionnelles à la série :

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Moyenne sur un million de points</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Temps</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Allocation</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;">Structure, puis extraction en <span><code>[</code></span><span><code>]</code></span><code>float64</code></td>
<td style="text-align: left;">368 µs</td>
<td style="text-align: left;"><strong>8 Mo par appel</strong></td>
</tr>
<tr>
<td style="text-align: left;">NaN-boxing, tableau passé tel quel</td>
<td style="text-align: left;">µs</td>
<td style="text-align: left;"><strong>aucune</strong></td>
</tr>
</tbody>
</table>

Deux fois plus lent, et 8 Mo alloués à chaque appel. Sur un traitement qui enchaîne moyenne, médiane, écart-type et percentiles, la facture se paie quatre fois.

#### 3. Elle coûte à chaque calcul

Chaque fonction qui travaille sur les mesures doit connaître la structure et tester le booléen. Le traitement des absences se répète partout au lieu d’être porté par la valeur elle-même.

#### 4. Elle se contourne sans que rien ne proteste

Le compilateur n’oblige jamais à lire `Valid`. Rien n’empêche d’écrire `v.Value` sur un relevé absent : on obtient zéro, un zéro qui a l’air d’une mesure et qui traverse tout le calcul sans laisser de trace. L’oubli n’est signalé ni à la compilation ni à l’exécution, et il se glisse dans la première fonction écrite un jour de fatigue.

Un trou, lui, se défend tout seul. Il n’existe aucune façon de lire « la valeur derrière le NaV » : la valeur *est* le NaV, et toute opération qui l’ignore le propage ou l’écarte selon les règles du paquet. Le mauvais usage n’est pas rendu difficile, il est rendu impossible.

#### 5. Un pointeur coûte davantage

`*float64`, avec `nil` pour l’absence, semble élégant. Mais chaque point devient un pointeur de 8 octets **plus** la valeur pointée quelque part ailleurs, et surtout : un tableau d’un million de pointeurs est parcouru par le ramasse-miettes à chaque cycle, alors qu’un `[``]``float64` ne contient aucun pointeur et lui reste totalement invisible.

#### 6. La valeur sentinelle est un piège

Coder l’absence par −999, ou par 0, est la solution la plus ancienne. Elle fonctionne jusqu’au jour où une vraie mesure vaut −999 — et ce jour arrive. Le NaV, lui, ne peut pas être confondu avec une mesure : aucune opération sur des nombres réels ne le produit.

#### Le tableau complet

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Solution</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Mémoire par point</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Compatible <span><code>[</code></span><span><code>]</code></span><code>float64</code></p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Invisible au GC</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Confusion possible</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><strong>NaN-boxing</strong></td>
<td style="text-align: left;">o</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;">non</td>
</tr>
<tr>
<td style="text-align: left;"><code>struct{float64; bool}</code></td>
<td style="text-align: left;">o</td>
<td style="text-align: left;">non</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;">non</td>
</tr>
<tr>
<td style="text-align: left;"><span><code>[</code></span><span><code>]</code></span><code>float64</code> + <span><code>[</code></span><span><code>]</code></span><code>bool</code></td>
<td style="text-align: left;">o</td>
<td style="text-align: left;">oui, mais le masque suit à part</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;">non</td>
</tr>
<tr>
<td style="text-align: left;"><code>*float64</code></td>
<td style="text-align: left;">o + la valeur</td>
<td style="text-align: left;">non</td>
<td style="text-align: left;"><strong>non</strong></td>
<td style="text-align: left;">non</td>
</tr>
<tr>
<td style="text-align: left;">Sentinelle −999</td>
<td style="text-align: left;">o</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;">oui</td>
<td style="text-align: left;"><strong>oui</strong></td>
</tr>
</tbody>
</table>

La vitesse de parcours, elle, ne départage pas : les quatre premières tournent entre 0,6 et 0,8 ms par million de points, et l’écart est dominé par le reste du calcul. **Ce qui décide, c’est la mémoire et la compatibilité**, pas les nanosecondes.

### Un avertissement

N’utilisez pas les opérateurs ordinaires sur des valeurs susceptibles d’être absentes. La charge utile que porte un NaN résultat est laissée au processeur, et les processeurs ne s’accordent pas : sur certains, `NaV - 5` ressort encore marqué comme NaV, sur d’autres le repère est perdu. Passez par les `Add`, `Sub`, `Mul`, `Div` et les agrégats de `notavalue`, qui font du résultat une propriété du code et non de la machine qui l’exécute.

## Le temps

### Comment un instant est stocké

Les horodatages sont des `time.Time`, conservés tels que donnés, fuseau compris. Un point pèse donc 32 octets là où un compte de nanosecondes en prendrait 16. La bibliothèque standard de Go a été déterminante dans le choix du langage pour le traitement des données temporelles.`time.Time` est un type bien développé en Go — fuseaux, formatage, comparaison, bibliothèque standard. La bibliothèque ne convertit jamais une série dans un autre fuseau de sa propre initiative : le fuseau que porte un relevé est une affirmation sur l’endroit où il a été pris.

### Le temps Unix, sous les horodatages

Sous la surface, tous les systèmes qui échangent des dates s’accordent sur une même origine : **le 1ᵉʳ janvier 1970 à 00:00:00 UTC**. Un instant s’y ramène à un seul nombre, le compte de ce qui s’est écoulé depuis — secondes, millisecondes ou nanosecondes selon la précision retenue. C’est le *temps Unix*, et c’est ce que `time.Time` manipule en interne, ce que PostgreSQL stocke, ce que transportent les capteurs.

Quelques repères pour lire un tel nombre :

| Nombre                      | Unité        | Instant                        |
|:----------------------------|:-------------|:-------------------------------|
| `0`                         | seconde      | 1ᵉʳ janvier 1970, 00:00:00 UTC |
| `1 000 000 000`             | secondes     | 9 septembre 2001               |
| `1 767 225 600`             | secondes     | 1ᵉʳ janvier 2026               |
| `1 767 225 600 000 000 000` | nanosecondes | le même instant                |

Trois propriétés méritent d’être connues, parce qu’elles expliquent des choix de la bibliothèque.

**Le temps Unix ne connaît pas les fuseaux.** C’est un compte depuis une origine, donc un instant absolu. Deux capteurs, l’un à Luxembourg et l’autre à Tokyo, qui mesurent au même moment produisent le même nombre. Le fuseau n’intervient qu’à l’affichage — et c’est exactement pourquoi une grille régulière s’aligne dessus (§5.6) : c’est la seule référence que deux séries partagent, où qu’elles aient été enregistrées.

**Il ignore les secondes intercalaires.** La rotation de la Terre n’est pas régulière, et l’UTC y ajoute de temps en temps une seconde — la dernière en 2016. Le temps Unix, lui, fait comme si elles n’existaient pas : une journée y compte toujours exactement 86 400 secondes. Les systèmes qui doivent rester à l’heure les absorbent en étirant imperceptiblement leur horloge sur quelques heures. Conséquence pratique : une durée calculée entre deux instants séparés par une seconde intercalaire est fausse d’une seconde, ce qui n’a d’importance que pour la métrologie fine.

**Il ne dit rien de la précision de la mesure.** Un horodatage à la nanoseconde n’implique pas que le capteur sache ce qu’il faisait à la nanoseconde près. Le nombre est exact, la mesure ne l’est pas forcément — et la bibliothèque conserve ce qu’on lui donne sans prétendre l’améliorer.

En Go, un `time.Time` porte davantage qu’un simple compte : l’instant absolu, un fuseau, et parfois une lecture d’horloge monotone — insensible aux changements d’heure du système — que la bibliothèque standard utilise pour mesurer des durées. Les horodatages venus d’une base ou d’un capteur n’en ont pas ; c’est sans conséquence ici.

### RFC 3339, la forme écrite

Le temps Unix est un nombre ; il faut aussi une forme écrite, lisible par un humain et non ambiguë pour une machine. C’est la **RFC 3339**, et c’est elle que produit `ToJSON` :

    2026-01-15T14:30:00Z           ← en UTC, le « Z » pour zéro décalage
    2026-01-15T15:30:00+01:00      ← le même instant, vu de Paris
    2026-01-15T14:30:00.123456789Z ← avec ses nanosecondes

La forme est stricte, et c’est ce qui en fait la valeur : une date, un `T`, une heure, un décalage. Du plus grand au plus petit, toujours, avec des zéros de remplissage. C’est un sous-ensemble volontairement réduit de la norme ISO 8601, qui autorise elle une foule de variantes — les semaines, les durées, les dates partielles, l’omission des séparateurs — et qu’aucune implémentation ne couvre entièrement.

Trois propriétés justifient de s’y tenir :

**Le tri lexicographique est le tri chronologique.** Deux horodatages RFC 3339 exprimés dans le même décalage se comparent comme du texte ordinaire, caractère par caractère, et l’ordre obtenu est le bon. C’est ce qui permet de trier un fichier de journaux avec `sort`, ou d’indexer une colonne texte sans la convertir.

**Le décalage est obligatoire.** Un horodatage sans décalage — comme en produisent tant de bases de données et d’API — ne désigne pas un instant : `2026-01-15 14:30:00` peut être quatorze heures et demie à Paris, à Tokyo ou à New York, soit trois instants distants de plusieurs heures. La RFC 3339 (Klyne and Newman 2002) l’interdit. Quand une donnée arrive dans cette forme, il faut lui adjoindre le fuseau que le fournisseur sous-entend, et c’est une décision, pas une conversion.

**Mais un décalage n’est pas un fuseau.** `+01:00` dit de combien l’heure locale s’écarte de l’UTC à cet instant précis ; il ne dit pas qu’on est à Paris, ni si l’heure d’été s’appliquait. Une série sérialisée en RFC 3339 puis relue perd donc le nom de son fuseau : elle garde l’instant exact, ce qui suffit à tout calcul, mais un regroupement calendaire effectué après ce trajet retombera sur un décalage figé plutôt que sur un vrai fuseau. **Quand les journées locales comptent, regroupez avant de sérialiser**, pas après.

### Une durée qui n’existe pas

Le premier point d’une série n’a pas de prédécesseur, donc l’intervalle qui le précède n’existe pas. C’est `NaDuration`, le pendant de NaV pour le temps. Il s’affiche « NaDuration » plutôt que sous la forme absurde de −2562047h47m16s, et `IsNaDuration` le reconnaît.

### Deux conventions différentes

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
&#10;</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Fenêtre</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Un relevé sur la frontière</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>L’instant émis</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><code>Regularize</code></td>
<td style="text-align: left;">fermée à droite</td>
<td style="text-align: left;">appartient à la fenêtre qui s’y <strong>termine</strong></td>
<td style="text-align: left;">le top</td>
</tr>
<tr>
<td style="text-align: left;"><code>Downscale*</code></td>
<td style="text-align: left;">ouverte à gauche</td>
<td style="text-align: left;"><strong>ouvre</strong> la nouvelle journée</td>
<td style="text-align: left;">le dernier instant de la période</td>
</tr>
</tbody>
</table>

`Regularize` convient au calcul : toutes ses fenêtres durent exactement la même chose, donc deux points pèsent toujours pareil. `Downscale` convient au calendrier : un relevé pris à minuit appartient à la journée qui commence, comme le dirait n’importe qui.

Les deux datent leur point à la **fin** de la période, si bien que tous deux se lisent « tout ce qui précède jusqu’ici ».

### Une grille régulière s’aligne sur l’UTC

La grille ne s’aligne pas sur le premier relevé — ce qui rendrait deux séries incomparables — mais sur le temps absolu, c’est-à-dire sur l’UTC.

Dans un fuseau décalé d’un nombre entier d’heures, c’est invisible :

    Paris (UTC+01:00), relevé à 10:47 → fenêtre horaire à 10:00 locales

Dans un fuseau décalé d’une demi-heure, ça ne l’est pas :

    Calcutta (UTC+05:30), relevé à 10:47 → fenêtre horaire à 10:30 locales

Ce sont les mêmes instants en UTC, et c’est précisément l’objectif : deux séries régularisées au même pas tombent sur les mêmes instants, où qu’elles aient été enregistrées. Si ce sont les frontières d’heures locales qui comptent — un rapport quotidien pour une équipe sur place —, utilisez `DownscaleDaily`, qui travaille dans le calendrier, ou décalez les horodatages avant de régulariser.

### Les périodes calendaires, et les jours qui ne durent pas 24 heures

Les frontières du calendrier n’ont de sens qu’en un lieu : la famille `Downscale` les calcule donc dans le fuseau du premier relevé de la série.

Elle traite les deux jours de l’année où les horloges changent — 23 heures au printemps, 25 à l’automne — et les fuseaux où **minuit n’existe pas** : à Santiago, La Havane et aux Açores, les horloges avancent à minuit, et la journée s’ouvre à 01:00. Bâtir une période sur un minuit qui n’a jamais eu lieu la fermerait une heure trop tôt, et toutes les périodes suivantes avec elle.

### La précision

Les statistiques sur les horodatages sont calculées sur des écarts au premier point, et non sur des nanosecondes depuis 1970.

Un `float64` porte 53 bits de mantisse ; un horodatage Unix actuel en nanosecondes en demande 61. Calculer sur des valeurs absolues arrondit à quelques centaines de nanosecondes — de quoi faire tomber au mauvais endroit l’instant moyen de trois relevés espacés de deux nanosecondes. Compter depuis le début de la série garde les nombres petits et le résultat exact.

## Une série

### Les types

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

### L’invariant

**À tout instant, les points sont dans l’ordre chronologique et chaque delta s’accorde avec cet ordre.**

Aucune méthode ne trie une série ni ne recalcule ses deltas : il n’y a jamais rien à réparer. `Add` et `AddBatchData` maintiennent l’invariant en insérant — y compris quand un relevé arrive en retard, ce qui décale la fin du tableau et recalcule exactement deux deltas.

D’où le champ privé. Livrer le tableau permettrait d’ajouter hors ordre ou de trier par mesure, laissant les deltas décrire un ordre disparu. La version précédente portait pour cela un drapeau, `deltasValid`, qui prévenait que son propre état pouvait mentir. Rendre l’état impossible vaut mieux que le signaler.

La lecture passe par `Len`, `At`, `First`, `Last`, `Range`, `Meas` et `MeasTo`.

### Le chargement, et ce qu’il coûte

| Un million de relevés en désordre | Temps        |
|:----------------------------------|:-------------|
| `Add`, un par un                  | ~2,5 minutes |
| `AddBatchData`                    | ~1 seconde   |

`Add` décale la fin du tableau pour chaque relevé qui doit s’insérer plus tôt, ce qui est quadratique. Mesuré : 61 ms pour 20 000 points, 255 ms pour 40 000, 1 014 ms pour 80 000 — un quadruplement à chaque doublement.

`AddBatchData` ajoute tout, trie une fois et remplit les deltas en une passe : 9, 21 et 45 ms sur les mêmes lots. Quand le lot prolonge déjà la série dans l’ordre — une requête avec `ORDER BY` —, il le constate en une passe et saute le tri.

`Add` pour le relevé qui arrive seul d’un flux, `AddBatchData` pour tout ce qu’on a déjà en main.

### Alimenter les agrégats sans allouer

    var buf []float64
    for _, ts := range all {
        buf = ts.MeasTo(buf[:0])           // aucune allocation après la première série
        fmt.Println(ts.Name, nav.Mean(buf))
    }

`Meas()` alloue un tableau neuf et reste la forme commode ; `MeasTo` remplit un tampon que vous gardez, ce qui compte dès qu’on boucle sur des centaines de séries.

## Le résumé

`Stats()` renvoie trente champs, calculés à la demande et jamais mis en cache — un résumé ne peut donc jamais décrire une série qui a changé depuis, et deux appels s’accordent toujours.

Les noms suivent une clé : `Ch*` concernent les horodatages, `Ms*` les mesures, `DCh*` et `DMs*` les deltas, et `ChAt*` donnent l’instant auquel une autre statistique se produit.

Regroupés par la question à laquelle ils répondent :

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Question</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Champs</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;">Sur quoi la série s’étend-elle ?</td>
<td style="text-align: left;"><code>Chmin</code>, <code>Chmax</code>, avec <code>ValAtChmin</code> et <code>ValAtChmax</code></td>
</tr>
<tr>
<td style="text-align: left;">Où commencent les données ?</td>
<td style="text-align: left;"><code>ChFirstUsable</code>, <code>ValAtFirstUsable</code></td>
</tr>
<tr>
<td style="text-align: left;">Où se situent les points ?</td>
<td style="text-align: left;"><code>Chmean</code>, <code>Chmed</code></td>
</tr>
<tr>
<td style="text-align: left;">Qu’a-t-on mesuré ?</td>
<td style="text-align: left;"><code>Msmin</code>, <code>Msmax</code> avec leurs instants, <code>Msmean</code>, <code>Msmed</code>, <code>Msstd</code></td>
</tr>
<tr>
<td style="text-align: left;">À quelle cadence arrivent les relevés ?</td>
<td style="text-align: left;"><code>DChmin</code>, <code>DChmax</code> avec leurs instants, <code>DChmean</code>, <code>DChmed</code>, <code>DChstd</code></td>
</tr>
<tr>
<td style="text-align: left;">De combien ça bouge ?</td>
<td style="text-align: left;"><code>DMsmin</code>, <code>DMsmax</code>, <code>DMsmean</code>, <code>DMsmed</code>, <code>DMsstd</code></td>
</tr>
<tr>
<td style="text-align: left;">Quelle est la qualité des données ?</td>
<td style="text-align: left;"><code>Len</code>, <code>NbreOfNaN</code>, <code>NbreOfNaV</code></td>
</tr>
</tbody>
</table>

**`Chmin` face à `ChFirstUsable`.** Le premier dit où la fenêtre s’ouvre, le second où les relevés commencent. Sur un capteur qui chauffait encore, les deux diffèrent, et les confondre date la série trop tôt.

**`NbreOfNaN` moins `NbreOfNaV`** donne le nombre de véritables erreurs de calcul. Quand les deux sont égaux, tout ce qui n’est pas un nombre dans la série est un trou, et rien n’est cassé.

**`DChstd`** mesure la régularité de l’échantillonnage : proche de zéro sur un flux discipliné, il grimpe dès qu’il bégaie. C’est souvent le nombre le plus instructif du tableau, parce qu’un intervalle moyen masque complètement une dérive — dix-neuf intervalles de 56 à 66 minutes ont une moyenne d’exactement une heure.

Une statistique qui ne peut pas être calculée vaut NaV, jamais zéro. Un relevé unique n’a pas d’écart-type ; une série de trous n’a pas de moyenne.

## Le nettoyage

### Ce qu’est un rejet

Un point rejeté n’est ni une erreur ni une suppression : c’est un relevé que l’appelant a décidé de ne plus croire. Il devient un trou et garde sa place dans le temps.

La version précédente remplaçait les points rejetés par un NaN ordinaire : une valeur aberrante dans un mois rendait NaN toutes les statistiques de ce mois. Avec un trou, la moyenne continue sur ce qui reste, et les compteurs disent combien a été écarté.

Les relevés rejetés reviennent dans une série à part, avec leurs valeurs d’origine : rien n’est perdu, et la décision reste révisable.

### Ce qui n’est jamais rejeté

- **Un trou** : il n’y avait rien à juger.

- **Une valeur cassée** : le nettoyage traite des mesures invraisem- blables, et déguiser une erreur en absence revient à la perdre.

Ni l’un ni l’autre ne participent au calcul des seuils.

### Les quatre méthodes

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Méthode</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Seuil</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>À utiliser quand</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><code>RemoveOutbounds(min, max)</code></td>
<td style="text-align: left;">Fixe</td>
<td style="text-align: left;">L’invraisemblable est connu d’avance : une pluie négative, une température au-dessus de l’ébullition</td>
</tr>
<tr>
<td style="text-align: left;"><code>RemovePercentileOutliers(low, high)</code></td>
<td style="text-align: left;">Percentiles des relevés</td>
<td style="text-align: left;">L’échelle est inconnue et les queues sont suspectes</td>
</tr>
<tr>
<td style="text-align: left;"><code>RemoveZScoreOutliers(level)</code></td>
<td style="text-align: left;">Moyenne ± level × écart-type</td>
<td style="text-align: left;">Les relevés se dispersent symétriquement ; 3 est l’usage</td>
</tr>
<tr>
<td style="text-align: left;"><code>RemovePeirceOutliers()</code></td>
<td style="text-align: left;">Critère de Peirce</td>
<td style="text-align: left;">Aucun seuil ne s’impose</td>
</tr>
</tbody>
</table>

Les bornes s’expriment en `float64`, avec `NaV` pour « pas de borne de ce côté » :

    ts.RemoveOutbounds(0, nav.NaV)            // rien en dessous de zéro
    ts.RemovePercentileOutliers(nav.NaV, 95)  // couper le haut seulement

**Sur les percentiles :** ils décrivent les relevés dont on dispose, donc les seuils bougent avec les données et quelque chose est toujours rejeté. C’est un outil différent des bornes fixes, pas un meilleur.

**Sur le critère de Peirce :** il ne prend aucun seuil. Il le déduit de la taille de l’échantillon, à travers une table publiée par Benjamin Peirce en 1852 (Peirce 1852; Ross 2003), et décide combien de relevés un échantillon de cette taille peut légitimement perdre. Il est plus sévère sur les petits échantillons, rejette au plus neuf relevés quelle que soit la taille, et — le test qui compte — ne rejette rien du tout d’une série saine. Sur l’exemple classique d’enseignement, il condamne exactement les deux relevés bas, 90 et 89.

## La régularisation

### La grille

    hourly, err := ts.Regularize(time.Hour, timeseries.AggMean)

Les fenêtres ferment à droite et s’alignent sur l’horloge (§5.6). Une fenêtre qui n’a rien reçu devient un trou : une grille régulière a un point par pas, et un pas sans relevé reste un pas. Rien n’est émis avant le premier relevé ni après le dernier — une série ne dit rien de ce qui précède son début ni de ce qui suit sa fin.

### La tolérance

Un enregistreur censé rendre son relevé à l’heure pile et qui le rend à 10:00:04 n’a pas manqué sa fenêtre. Sans tolérance, il tombe une fenêtre trop tard :

| Fenêtre | Sans tolérance                         | 5 minutes de tolérance |
|:--------|:---------------------------------------|:-----------------------|
| :00     | 57                                     | 57                     |
| 02:00   | **NaV**                                | 123                    |
| 03:00   | **151,5** — la moyenne de deux relevés | 180                    |
| 04:00   | **NaV**                                | 241                    |

Le signal horaire devient une alternance de trous et de moyennes fausses. La tolérance doit rester inférieure au pas, sans quoi un relevé appartiendrait à deux fenêtres.

### Les agrégateurs

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Agrégateur</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Renvoie</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Pour</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><code>AggMean</code></td>
<td style="text-align: left;">La moyenne de la fenêtre</td>
<td style="text-align: left;">Une grandeur physique</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggMedian</code></td>
<td style="text-align: left;">La médiane</td>
<td style="text-align: left;">Idem, quand un relevé égaré ne doit pas tirer le résultat</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggMin</code>, <code>AggMax</code></td>
<td style="text-align: left;">Les extrêmes</td>
<td style="text-align: left;">Des enveloppes</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggSum</code></td>
<td style="text-align: left;">La somme</td>
<td style="text-align: left;">Une grandeur qui s’accumule : pluie, énergie, événements</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggFirst</code>, <code>AggLast</code></td>
<td style="text-align: left;">Le relevé du bord, trou compris</td>
<td style="text-align: left;">Un signal d’état, un compteur</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggFirstUsable</code>, <code>AggLastUsable</code></td>
<td style="text-align: left;">Le relevé du bord, en enjambant les trous</td>
<td style="text-align: left;">Une grandeur physique</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggCountUsable</code></td>
<td style="text-align: left;">Combien de relevés sont arrivés</td>
<td style="text-align: left;">Un rapport de couverture</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggSlope</code></td>
<td style="text-align: left;">La pente de la droite ajustée</td>
<td style="text-align: left;">Une tendance par pas</td>
</tr>
<tr>
<td style="text-align: left;"><code>AggIntegral(step)</code></td>
<td style="text-align: left;">L’aire sous la fenêtre</td>
<td style="text-align: left;">Transformer un débit en quantité</td>
</tr>
</tbody>
</table>

Tous suivent la politique NaV : un trou est ignoré, une erreur se propage. Une fenêtre vide n’atteint jamais l’agrégateur — elle est émise en NaV directement.

`Aggregator("maximum")` en renvoie un par son nom, pour qu’une recette stockée en base puisse le choisir. Un nom inconnu est une erreur, jamais un repli silencieux sur la moyenne.

## Le regroupement calendaire

`DownscaleDaily`, `DownscaleWeekly`, `DownscaleMonthly` et `DownscaleYearly` regroupent par période du calendrier.

**Ces méthodes servent à montrer, pas à calculer.** Les périodes calendaires cachent une durée variable derrière un nom familier :

- un mois dure de 28 à 31 jours — février contient environ 10 % de temps en moins que mars ;

- une année dure 365 ou 366 jours ;

- une journée dure 24 heures, sauf les deux où les horloges changent.

Deux points mensuels ne pèsent donc pas la même chose. Un agrégateur qui croît avec la durée de la fenêtre — `AggSum`, `AggCountUsable`, `AggIntegral` — produit des nombres qui diffèrent en partie parce que les périodes diffèrent, et février ressort plus bas pour la seule raison qu’il est plus court.

**Régulariser pour calculer, regrouper en dernier, pour montrer.**

## Comprimer et restituer

### Reduce

Un signal qui tient sa valeur entre deux changements — une porte, une consigne, une machine à états — stocke le même nombre encore et encore. `Reduce` garde le premier point, le dernier, et les changements entre les deux.

Ce qui compte comme changement suit la politique plutôt qu’une comparaison ordinaire, puisqu’aucun NaN n’est égal à lui-même : deux trous consécutifs ne sont pas un changement, mais entrer dans un trou ou en sortir en est un — et c’est souvent l’information la plus intéressante qu’une série enregistre.

`ReduceWithDeadband(band)` fait de même avec une tolérance : un relevé n’est gardé que s’il diffère **du dernier relevé gardé** de plus que la bande — et non du relevé précédent, si bien qu’une dérive lente est attrapée dès qu’elle a bougé de plus que la bande au total. Celle-là perd de l’information, d’une quantité bornée, et le dit dans son nom.

### Expand

`Expand(start, end, step)` reconstruit une grille régulière en tenant chaque valeur jusqu’au changement suivant. Avant le premier point connu, il rend NaV : la série ne dit rien de ce que faisait le signal à ce moment-là, et un trou vaut mieux qu’une valeur inventée.

### MarkSilences

Réduire une série brute comporte un piège : si l’instrument cesse d’émettre pendant une journée, rien dans la série réduite n’enregistre le silence — la dernière valeur semble simplement se maintenir.

`MarkSilences(maxGap)` insère un NaV partout où deux relevés sont espacés de plus de `maxGap`, daté `maxGap` après le dernier relevé précédant le silence — le moment où une alarme surveillant le capteur se serait déclenchée. Jusque-là, la valeur tenue est légitime : un capteur qui émet toutes les dix minutes n’est pas perdu à la onzième. Le silence survit alors à la réduction, sans passer par une grille.

## L’interpolation

Seule opération qui invente des données : ce qui en sort est une lecture de ce qui s’est probablement passé, pas un relevé.

### Sur le temps, pas sur les rangs

Un trou est comblé d’après sa place **dans le temps**. Sur une série régularisée, ça ne change rien ; sur une série brute, ça change tout :

    10 mesuré à 00:00, un trou à 00:01, 20 mesuré à 01:00

       par le temps (cette bibliothèque) : 10,17
       par les rangs (l'ancienne)        : 15

Le trou est à une minute de son voisin de gauche et à cinquante-neuf de celui de droite. Quinze n’est pas une réponse défendable.

### Les sept méthodes

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Méthode</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Comble avec</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Pour</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><code>InterpLinear</code></td>
<td style="text-align: left;">Une droite dans le temps</td>
<td style="text-align: left;">Une grandeur qui varie continûment</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpNearest</code></td>
<td style="text-align: left;">Le voisin le plus proche</td>
<td style="text-align: left;">Un signal en marches ; garde une valeur mesurée</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpForwardFill</code></td>
<td style="text-align: left;">La dernière valeur connue</td>
<td style="text-align: left;">Une consigne, un état, un compteur</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpBackwardFill</code></td>
<td style="text-align: left;">La valeur connue suivante</td>
<td style="text-align: left;">Le début d’une série</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpLogLinear</code></td>
<td style="text-align: left;">Un taux de croissance constant</td>
<td style="text-align: left;">Une grandeur qui se compose ; les deux voisins doivent être positifs</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpCubicSpline</code></td>
<td style="text-align: left;">Une spline cubique naturelle</td>
<td style="text-align: left;">La régularité avant tout</td>
</tr>
<tr>
<td style="text-align: left;"><code>InterpMonotoneSpline</code></td>
<td style="text-align: left;">Une spline PCHIP <span class="citation" data-cites="fritsch1980">(Fritsch and Carlson 1980)</span></td>
<td style="text-align: left;">La régularité sans dépassement</td>
</tr>
</tbody>
</table>

**Pourquoi la spline monotone existe.** À travers une marche de 0 à 10, la spline naturelle comble le trou suivant avec **14,44** — une valeur supérieure à tous les relevés de la série, qu’aucun instrument n’a vue. La spline monotone reste à 10. Sur des données mesurées, c’est ce qui en fait la plus sûre des deux.

### Seulement les trous, et seulement les courts

Les valeurs cassées sont laissées telles quelles (§4.4), et rien n’extrapole : un trou sans relevé d’un côté reste un trou, à l’exception des deux reports, qui s’appuient sur un seul voisin par construction.

Aucune méthode ne sait quelle durée de panne peut être franchie. Une demi-heure de température manquante se traverse ; trois jours, non — et un graphique qui cache la panne derrière une courbe lisse ment sans en avoir l’air :

    filled, err := ts.InterpolateWithin(timeseries.InterpLinear, 2*time.Hour)

La limite se mesure entre les relevés qui encadrent le trou : elle signifie donc la même chose sur une série brute et sur une grille.

## Les conteneurs

Un `TsContainer` garde les variantes d’un même signal : `raw`, `cleaned`, `hourly`, tout ce qu’une recette a produit.

Il les conserve **dans l’ordre où elles ont été déposées**, ce qu’une map Go ne fait pas : son parcours est aléatoire. La version précédente imprimait donc les variantes dans un ordre différent à chaque exécution, et une légende de graphique se remélangeait entre deux appels. Remplacer une variante lui conserve son rang.

`Get` distingue un nom jamais déposé d’un nom déposé à `nil` : le premier n’a jamais été demandé, le second n’a pas pu être produit, et l’affichage dit lequel.

## Les sorties

### Terminal

    ts.PrettyPrint()       // le tableau des points
    ts.PrintStats()        // le résumé, en quatre sections
    ts.PrettyPrint(90, 95) // une fenêtre, pour une longue série

Les sentinelles s’affichent par leur nom — `NaV`, `NaN`, `NaDuration`, un tiret pour un instant inconnu. Un trou affiché « NaN », ou sous la forme d’une date de l’an 1, est un trou que personne ne remarque.

Les variantes `Fprint*` prennent un `io.Writer`, pour un journal ou un test.

### JSON

`ToJSON` produit la forme que lit un frontend : des colonnes plutôt que des objets.

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Clé</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Contenu</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;"><code>chron</code></td>
<td style="text-align: left;">Les instants, en RFC 3339</td>
</tr>
<tr>
<td style="text-align: left;"><code>meas</code>, <code>dmeas</code></td>
<td style="text-align: left;">Mesures et variations, les non-nombres en <code>null</code></td>
</tr>
<tr>
<td style="text-align: left;"><code>dchron_ns</code></td>
<td style="text-align: left;">Les intervalles en nanosecondes, <code>NaDuration</code> en <code>null</code></td>
</tr>
<tr>
<td style="text-align: left;"><code>stats</code></td>
<td style="text-align: left;">Le résumé, mêmes noms de champs que <code>BasicStats</code>, en minuscules</td>
</tr>
</tbody>
</table>

Tout non-nombre devient `null`, JSON n’ayant pas de NaN. **La distinction entre un trou et une erreur ne survit donc pas à la conversion** — ce sont les compteurs `nbreOfNaV` et `nbreOfNaN` du résumé qui la transportent.

## Les performances, mesurées

Sur un portable Apple à processeur M.

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

La croissance est linéaire jusqu’à dix millions de points pour tout ce qui n’est pas médiane, celles-ci portant leur `n log n`.

## Venir de la version précédente

La bibliothèque a été réécrite pour la `v0.2`. Ce qui a changé, et pourquoi :

<table>
<thead>
<tr>
<th style="text-align: left;"><div class="minipage">
<p>Avant</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Maintenant</p>
</div></th>
<th style="text-align: left;"><div class="minipage">
<p>Pourquoi</p>
</div></th>
</tr>
</thead>
<tbody>
<tr>
<td style="text-align: left;">Un point rejeté devenait <code>NaN</code></td>
<td style="text-align: left;">Il devient <code>NaV</code></td>
<td style="text-align: left;">Une valeur aberrante ne détruit plus un mois de statistiques</td>
</tr>
<tr>
<td style="text-align: left;">Statistiques stockées dans la série</td>
<td style="text-align: left;"><code>Stats()</code> calcule à la demande</td>
<td style="text-align: left;">Des statistiques stockées se périmaient en silence</td>
</tr>
<tr>
<td style="text-align: left;">Un drapeau <code>deltasValid</code></td>
<td style="text-align: left;">Un invariant</td>
<td style="text-align: left;">Rendre le mauvais état impossible vaut mieux que le signaler</td>
</tr>
<tr>
<td style="text-align: left;"><code>DataSeries</code> public</td>
<td style="text-align: left;">Privé, avec des accesseurs</td>
<td style="text-align: left;">Ce drapeau n’existait que parce que le tableau était public</td>
</tr>
<tr>
<td style="text-align: left;">Tolérance acceptée, jamais appliquée</td>
<td style="text-align: left;">Appliquée</td>
<td style="text-align: left;">Elle était silencieusement ignorée</td>
</tr>
<tr>
<td style="text-align: left;">Interpolation par rangs</td>
<td style="text-align: left;">Par le temps</td>
<td style="text-align: left;">Le rang est faux sur toute série irrégulière</td>
</tr>
<tr>
<td style="text-align: left;">Interpolation modifiant en place</td>
<td style="text-align: left;">Rend une nouvelle série</td>
<td style="text-align: left;">Cohérent avec le nettoyage et la régularisation</td>
</tr>
<tr>
<td style="text-align: left;"><code>MemId uint64</code></td>
<td style="text-align: left;"><code>ID string</code></td>
<td style="text-align: left;">Il n’était jamais rempli ; une chaîne accepte une clé de base ou un UUID</td>
</tr>
<tr>
<td style="text-align: left;"><code>Chstd</code></td>
<td style="text-align: left;">Supprimé</td>
<td style="text-align: left;">La dispersion des horodatages ne dit rien ; <code>DChstd</code> dit l’utile</td>
</tr>
<tr>
<td style="text-align: left;">—</td>
<td style="text-align: left;"><code>ChFirstUsable</code></td>
<td style="text-align: left;">Là où la fenêtre s’ouvre n’est pas là où les données commencent</td>
</tr>
<tr>
<td style="text-align: left;">—</td>
<td style="text-align: left;"><code>NbreOfNaV</code></td>
<td style="text-align: left;">Distinguer les trous des erreurs</td>
</tr>
<tr>
<td style="text-align: left;">—</td>
<td style="text-align: left;"><code>InterpolateWithin</code>, <code>MarkSilences</code>, <code>ReduceWithDeadband</code></td>
<td style="text-align: left;">Nouveaux</td>
</tr>
</tbody>
</table>

Pour le contrat JSON : `id` est désormais une chaîne, `chstd` disparaît, et `nbreOfNaV`, `chFirstUsable` et `valAtFirstUsable` s’ajoutent.

## Où vit le raisonnement

Chaque décision ci-dessus est soutenue par un test qui l’énonce en toutes lettres et par un message de commit qui consigne pourquoi elle a été prise. Quand un comportement surprend, `git log` et `git blame` sur le fichier l’expliquent mieux que le code.

<div id="refs" class="references csl-bib-body hanging-indent">

<div id="ref-fritsch1980" class="csl-entry">

Fritsch, F. N., and R. E. Carlson. 1980. “Monotone Piecewise Cubic Interpolation.” *SIAM Journal on Numerical Analysis* 17 (2): 238–46. <https://doi.org/10.1137/0717021>.

</div>

<div id="ref-ieee754" class="csl-entry">

IEEE Standard for Floating-Point Arithmetic, Pub. L. Nos. IEEE Std 754-2019 (2019). <https://doi.org/10.1109/IEEESTD.2019.8766229>.

</div>

<div id="ref-rfc3339" class="csl-entry">

Klyne, Graham, and Chris Newman. 2002. *Date and Time on the Internet: Timestamps*. RFC 3339, IETF. <https://doi.org/10.17487/RFC3339>.

</div>

<div id="ref-peirce1852" class="csl-entry">

Peirce, Benjamin. 1852. “Criterion for the Rejection of Doubtful Observations.” *The Astronomical Journal* 2 (45): 161–63.

</div>

<div id="ref-ross2003" class="csl-entry">

Ross, Stephen M. 2003. “Peirce’s Criterion for the Elimination of Suspect Experimental Data.” *Journal of Engineering Technology* 20 (2): 38–41.

</div>

</div>
