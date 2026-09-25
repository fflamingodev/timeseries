-- Filtre pandoc pour le guide.
--
-- Le Markdown porte sa propre table des matières et numérote ses titres à
-- la main, ce que GitHub et Typora demandent. LaTeX sait faire les deux :
-- ce filtre retire donc la table des matières du corps et les numéros des
-- titres, pour laisser \tableofcontents et \section s'en charger.

local skipping = false

local function is_toc(header)
  local text = pandoc.utils.stringify(header)
  return text:match("^Table des mati") or text:match("^Contents$")
end

-- Retire « 3. » ou « 3.5 » en tête d'un titre.
local function strip_number(inlines)
  if #inlines >= 2 and inlines[1].t == "Str"
     and inlines[1].text:match("^%d+%.?%d*%.?$") and inlines[2].t == "Space" then
    table.remove(inlines, 1)
    table.remove(inlines, 1)
  end
  return inlines
end

-- Les filets horizontaux séparaient les chapitres dans le Markdown ;
-- en LaTeX, les titres s'en chargent.
function HorizontalRule()
  return {}
end

function Pandoc(doc)
  local blocks = {}
  for _, block in ipairs(doc.blocks) do
    if block.t == "Header" then
      skipping = is_toc(block)
      if not skipping then
        local text = pandoc.utils.stringify(block)
        -- LaTeX ne numérote que jusqu'au troisième niveau : en dessous,
        -- les numéros écrits à la main sont les seuls, on les garde.
        if block.level <= 3 then
          block.content = strip_number(block.content)
        end
        block.identifier = ""      -- les ancres du Markdown ne servent plus
        -- Le résumé ne prend pas de numéro : sans cela il deviendrait le
        -- chapitre 1 et décalerait toutes les références du texte.
        if text == "Résumé" or text == "Abstract" then
          block.classes = {"unnumbered"}
        end
        table.insert(blocks, block)
      end
    elseif not skipping then
      table.insert(blocks, block)
    end
  end
  doc.blocks = blocks
  return doc
end
