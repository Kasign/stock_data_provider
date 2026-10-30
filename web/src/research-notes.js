export const RESEARCH_NOTES_KEY = "stock-provider-webui.research-notes.v1";
export const MAX_RESEARCH_NOTES = 200;
const MAX_TITLE = 100, MAX_CONTENT = 10000, MAX_TAGS = 12;
function safeStorage(storage) { if (storage) return storage; try { return globalThis.localStorage; } catch { return null; } }
function normalizeNote(note) {
  if (!note || typeof note !== "object") return null;
  const id=String(note.id||"").trim(), title=String(note.title||"").trim(), content=String(note.content||"").trim();
  if (!id || !title || !content || title.length>MAX_TITLE || content.length>MAX_CONTENT) return null;
  const tags=Array.isArray(note.tags)?note.tags.map(x=>String(x).trim()).filter(Boolean).slice(0,MAX_TAGS):[];
  return {id,title,content,symbol:String(note.symbol||"").trim().slice(0,32),tags,createdAt:String(note.createdAt||""),updatedAt:String(note.updatedAt||"")};
}
export function readResearchNotes(storage) {
  try {
    const target=safeStorage(storage); if(!target) return {items:[],readable:false,valid:false};
    const raw=target.getItem(RESEARCH_NOTES_KEY);
    if(raw===null) return {items:[],readable:true,valid:true};
    const parsed=JSON.parse(raw);
    if(!Array.isArray(parsed)||parsed.length>MAX_RESEARCH_NOTES) return {items:[],readable:true,valid:false};
    const items=parsed.map(normalizeNote);
    if(items.some(x=>!x)) return {items:[],readable:true,valid:false};
    return {items,readable:true,valid:true};
  } catch { return {items:[],readable:false,valid:false}; }
}
export function writeResearchNotes(items, storage) {
  if(!Array.isArray(items)||items.length>MAX_RESEARCH_NOTES) return {ok:false,reason:"limit"};
  const normalized=items.map(normalizeNote);
  if(normalized.some(x=>!x)) return {ok:false,reason:"invalid"};
  try {
    const target=safeStorage(storage); if(!target) return {ok:false,reason:"unavailable"};
    target.setItem(RESEARCH_NOTES_KEY,JSON.stringify(normalized));
    const check=readResearchNotes(target);
    return check.readable&&check.valid&&check.items.length===normalized.length?{ok:true}:{ok:false,reason:"verify"};
  } catch { return {ok:false,reason:"unavailable"}; }
}
export function filterResearchNotes(items, query="", symbol="") {
  const q=String(query||"").trim().toLowerCase(), s=String(symbol||"").trim().toLowerCase();
  return (Array.isArray(items)?items:[]).filter(n=>(!s||n.symbol.toLowerCase().includes(s))&&(!q||[n.title,n.content,n.symbol,...n.tags].join(" ").toLowerCase().includes(q))).sort((a,b)=>String(b.updatedAt).localeCompare(String(a.updatedAt)));
}
export function bindResearchNotes({root=document, getContext=()=>({}), storage}={}) {
  const $=id=>root.querySelector("#"+id), form=$("#research-note-form"); if(!form)return;
  let editingId="";
  const feedback=(message,error=false)=>{const el=$("#research-note-feedback");el.textContent=message;el.dataset.state=error?"error":"ok";};
  const state=()=>readResearchNotes(storage);
  const render=()=>{
    const host=$("#research-note-list"), result=state(); host.replaceChildren();
    if(!result.readable||!result.valid){const p=document.createElement("p");p.className="table-empty";p.textContent="浏览器存储不可读取或数据格式异常；为保护原始记录，当前未修改数据。";host.append(p);return;}
    const items=filterResearchNotes(result.items,$("#research-note-search").value,$("#research-note-symbol-filter").value);
    if(!items.length){const p=document.createElement("p");p.className="table-empty";p.textContent=result.items.length?"没有匹配的研究记录。":"尚无研究记录。";host.append(p);return;}
    items.forEach(note=>{
      const card=document.createElement("article");card.className="research-note-card";
      const header=document.createElement("header"), title=document.createElement("h3"), meta=document.createElement("span");
      title.textContent=note.title;meta.className="research-note-meta";meta.textContent=[note.symbol||"未关联股票",note.updatedAt||""].filter(Boolean).join(" · ");header.append(title,meta);card.append(header);
      const body=document.createElement("p");body.textContent=note.content;card.append(body);
      if(note.tags.length){const tags=document.createElement("div");tags.className="research-note-tags";tags.textContent="标签："+note.tags.join("、");card.append(tags);}
      const actions=document.createElement("div");actions.className="research-note-card-actions";
      [["编辑","edit"],["删除","delete"]].forEach(([label,action])=>{const b=document.createElement("button");b.type="button";b.className="button button-secondary";b.textContent=label;b.dataset.noteAction=action;b.dataset.noteId=note.id;actions.append(b);});
      card.append(actions);host.append(card);
    });
  };
  form.addEventListener("submit",event=>{
    event.preventDefault();
    const title=$("#research-note-title").value.trim(),content=$("#research-note-content").value.trim();
    if(!title||!content){feedback("标题和研究内容不能为空。",true);return;}
    const current=state();if(!current.readable||!current.valid){feedback("存储不可用或数据异常，未写入。",true);return;}
    const now=new Date().toISOString(),existing=editingId?current.items.find(x=>x.id===editingId):null;
    const note={id:editingId||("note-"+now+"-"+Math.random().toString(36).slice(2,8)),title,content,symbol:$("#research-note-symbol").value.trim(),tags:$("#research-note-tags").value.split(/[,，、]/).map(x=>x.trim()).filter(Boolean).slice(0,MAX_TAGS),createdAt:existing?.createdAt||now,updatedAt:now};
    const next=existing?current.items.map(x=>x.id===editingId?note:x):[note,...current.items];
    if(!existing&&next.length>MAX_RESEARCH_NOTES){feedback("最多保存 200 条，请先整理或删除旧记录。",true);return;}
    const saved=writeResearchNotes(next,storage);if(!saved.ok){feedback("保存失败，原有记录未确认写入："+saved.reason,true);return;}
    editingId="";form.reset();$("#research-note-cancel").hidden=true;feedback("研究记录已保存在当前浏览器，不会同步到服务端。");render();
  });
  $("#research-note-cancel").addEventListener("click",()=>{editingId="";form.reset();$("#research-note-cancel").hidden=true;feedback("已取消编辑。");});
  $("#research-note-search").addEventListener("input",render);$("#research-note-symbol-filter").addEventListener("input",render);
  $("#research-note-list").addEventListener("click",event=>{
    const button=event.target.closest("[data-note-action]");if(!button)return;
    const current=state();if(!current.readable||!current.valid)return;
    const note=current.items.find(x=>x.id===button.dataset.noteId);if(!note)return;
    if(button.dataset.noteAction==="edit"){editingId=note.id;$("#research-note-title").value=note.title;$("#research-note-content").value=note.content;$("#research-note-symbol").value=note.symbol;$("#research-note-tags").value=note.tags.join(", ");$("#research-note-cancel").hidden=false;feedback("正在编辑记录。");form.scrollIntoView({behavior:"smooth",block:"center"});return;}
    if(!globalThis.confirm("确定删除这条研究记录？"))return;
    const saved=writeResearchNotes(current.items.filter(x=>x.id!==note.id),storage);feedback(saved.ok?"记录已删除。":"删除失败，未能确认存储结果。",!saved.ok);render();
  });
  $("#research-note-symbol").value=String(getContext()?.symbol||"");
  render();
}
