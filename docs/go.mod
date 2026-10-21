// Ce répertoire n'est pas du code Go, et ce fichier n'est pas là pour
// être compilé. Il est là parce qu'un répertoire qui contient un
// go.mod est un autre module, et qu'un autre module est exclu de
// l'archive que le proxy sert pour le module parent :
//
//     // Skip submodules (directories containing go.mod files).
//     return filepath.SkipDir
//                       -- golang.org/x/mod/zip, listFilesInDir
//
// Les sources du guide — 124 Ko de LaTeX, de bibliographie et de
// scripts — restent donc versionnées et reconstructibles après un
// clone, sans être téléchargées par qui importe la bibliothèque.
//
// Personne n'importe ce module. Le chemin n'est là que pour être
// valide.
module usefulrisk.com/timeseries/docs

go 1.21
