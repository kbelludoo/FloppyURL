#!/usr/bin/env python3
"""lint transpilador P0: HTML-subset -> .lay canonico. Stdlib only, deterministico, fail-closed.
Subset congelado (excelencia = restricao):
  tags: div,h1,h2,h3,p,pre,button,input,span,a,img,section + wrappers row/col/view
  attrs: id,class,placeholder,href,src,alt,ACTION/mount|clear|increment
  style: apenas via STYLE bg/fg/font/size/pad/margin/w/h/display/border/radius/align/gap/flex/weight/cursor/opacity
  texto: so texto puro (sem nested tags mistas)
Fora do subset -> REJEITA com erro (script, style, form complexo, css arbitrario, tabelas, etc).
Saida canonica: @LAY:1.0 + VIEW + indentacao 4 espacos, chaves ordenadas -> bytes identicos p/ mesma entrada.
"""
import re, sys
from html.parser import HTMLParser

LAY_TAGS={"DIV","ROW","COL","H1","H2","H3","P","PRE","BUTTON","INPUT","SPAN","A","IMG","IFRAME","SECTION"}
STYLE_KEYS={"bg","fg","font","size","pad","margin","w","h","display","border","radius","align","valign","gap","flex","weight","shadow","cursor","opacity"}
ACTIONS={"mount","clear","increment"}
CSS_MAP={"background":"bg","color":"fg","font-family":"font","font-size":"size","padding":"pad",
 "margin":"margin","width":"w","height":"h","display":"display","border":"border","border-radius":"radius",
 "text-align":"align","align-items":"valign","gap":"gap","flex-direction":"flex","font-weight":"weight",
 "text-shadow":"shadow","cursor":"cursor","opacity":"opacity"}

class Fail(Exception): pass

class Node:
    def __init__(self,tag,attrs,style,text=""):
        self.tag=tag; self.attrs=list(attrs); self.style=dict(style); self.text=text; self.kids=[]

class P(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.stack=[]; self.root=None; self.rejected=[]
    def handle_starttag(self,tag,attrs):
        t=tag.lower()
        if t in ("html","head","body","meta","title","link"): return  # wrapper ignorado
        if t in ("script","style","form","table","video","audio","canvas","iframe" if False else "never"):
            raise Fail(f"<{t}> fora do subset lint-P0 (comportamento/estilo arbitrario)")
        if t=="iframe":
            raise Fail("<iframe> fora do subset lint-P0")
        am=dict(attrs)
        # style inline -> traduz so chaves conhecidas, resto rejeita
        st={}
        if "style" in am:
            for part in am.pop("style").split(";"):
                part=part.strip()
                if not part: continue
                if ":" not in part: raise Fail(f"css invalido: {part!r}")
                k,v=[x.strip() for x in part.split(":",1)]
                lk=CSS_MAP.get(k.lower())
                if lk is None: raise Fail(f"css '{k}' fora do subset lint-P0")
                st[lk]=v
        # class row/col -> ROW/COL
        cls=(am.pop("class","") or "")
        lay="DIV"
        if t=="div" and "row" in cls.split(): lay="ROW"
        elif t=="div" and "col" in cls.split(): lay="COL"
        elif t in ("h1","h2","h3","p","pre","button","input","span","a","img","section"): lay=t.upper()
        elif t=="div": lay="DIV"
        else: raise Fail(f"<{t}> fora do subset lint-P0")
        rest=[]
        if cls and "row" not in cls.split() and "col" not in cls.split():
            rest.append(f"CLASS={cls}")
        for k in ("id","placeholder","href","src","alt"):
            if k in am: rest.append(f"{k}={am[k]}")
        for k in list(am):
            if k.lower()=="action":
                v=am[k]
                if v not in ACTIONS: raise Fail(f"ACTION={v!r} desconhecida (mount|clear|increment)")
                rest.append(f"ACTION={v}")
            elif k not in ("id","placeholder","href","src","alt","class"):
                raise Fail(f"attr '{k}' fora do subset lint-P0")
        n=Node(lay,rest,st)
        if self.stack: self.stack[-1].kids.append(n)
        else:
            if self.root is not None: raise Fail("multiplas raizes (P0 exige 1 VIEW)")
            self.root=n
        if t not in ("input","img"): self.stack.append(n)
    def handle_endtag(self,tag):
        t=tag.lower()
        if t in ("html","head","body"): return
        if self.stack and self.stack[-1].tag==t.upper(): self.stack.pop()
        elif self.stack and t=="div" and self.stack[-1].tag in ("ROW","COL","DIV"): self.stack.pop()
    def handle_data(self,data):
        s=data.strip()
        if not s: return
        if not self.stack: raise Fail(f"texto fora de no: {s[:30]!r}")
        if self.stack[-1].text: raise Fail("texto misto/nested fora do subset P0")
        if len(s)>500: raise Fail("texto >500 chars fora do P0")
        self.stack[-1].text=s

def emit(n,ind=0):
    pad="    "*ind
    if ind==0:
        lines=["@LAY:1.0",f"VIEW app"]
        if n.style: lines.append("STYLE "+" ".join(f"{k}={n.style[k]}" for k in sorted(n.style)))
        for k in n.kids: lines.extend(emit(k,1))
        lines.append("END")
        return lines
    line=f"{pad}{n.tag}"
    if n.text: line+=f' "{n.text}"'
    if n.style: line+=f" STYLE "+" ".join(f"{k}={n.style[k]}" for k in sorted(n.style))
    if n.attrs: line+=" "+" ".join(sorted(n.attrs))
    lines=[line]
    for k in n.kids: lines.extend(emit(k,ind+1))
    return lines

def transpile(html:str)->str:
    if re.search(r"<script[\s>]",html,re.I): raise Fail("<script> fora do subset lint-P0")
    if re.search(r"<style[\s>]",html,re.I): raise Fail("<style> bloco fora do subset lint-P0 (use STYLE inline do subset)")
    p=P(); p.feed(html); p.close()
    if p.root is None: raise Fail("nenhuma raiz layout encontrada")
    if p.stack: raise Fail("tags nao fechadas")
    return "\n".join(emit(p.root))+"\n"

if __name__=="__main__":
    src=open(sys.argv[1]).read() if len(sys.argv)>1 else sys.stdin.read()
    try: sys.stdout.write(transpile(src))
    except Fail as e: sys.stderr.write(f"lint-transpile FAIL-CLOSED: {e}\n"); sys.exit(1)
