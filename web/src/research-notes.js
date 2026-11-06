export const RESEARCH_NOTES_KEY = "stock-provider-webui.research-notes.v1";
export const MAX_RESEARCH_NOTES = 200;
const MAX_TITLE = 100, MAX_CONTENT = 10000, MAX_TAGS = 12, MAX_SYMBOLS = 100;
function safeStorage(storage) { if (storage) return storage; try { return globalThis.localStorage; } catch { return null; } }
function normalizeNote(note) {
  if (!note || typeof note !== "object" || Array.isArray(note)) return null;
  const id = typeof note.id === "string" ? note.id.trim() : "";
  const title = typeof note.title === "string" ? note.title.trim() : "";
  const content = typeof note.content === "string" ? note.content.trim() : "";
  const symbol = note.symbol === undefined ? "" : note.symbol;
  const symbols = note.symbols === undefined ? (typeof symbol === "string" && symbol.trim() ? [symbol.trim()] : []) : note.symbols;
  const taskId = note.taskId === undefined ? "" : note.taskId;
  const runId = note.runId === undefined ? "" : note.runId;
  const tags = note.tags === undefined ? [] : note.tags;
  const createdAt = note.createdAt === undefined ? "" : note.createdAt;
  const updatedAt = note.updatedAt === undefined ? "" : note.updatedAt;
  if (!id || id.length > 160 || !title || !content || title.length > MAX_TITLE || content.length > MAX_CONTENT ||
      typeof symbol !== "string" || symbol.trim().length > 32 ||
      !Array.isArray(symbols) || symbols.length > MAX_SYMBOLS ||
      symbols.some(value => typeof value !== "string" || !value.trim() || value.trim().length > 32) ||
      typeof taskId !== "string" || taskId.trim().length > 160 ||
      typeof runId !== "string" || runId.trim().length > 160 ||
      !Array.isArray(tags) || tags.length > MAX_TAGS ||
      tags.some(tag => typeof tag !== "string" || !tag.trim() || tag.trim().length > 40) ||
      typeof createdAt !== "string" || createdAt.length > 64 ||
      typeof updatedAt !== "string" || updatedAt.length > 64) return null;
  const normalizedSymbols = [...new Set(symbols.map(value => value.trim()))];
  const primarySymbol = symbol.trim() || normalizedSymbols[0] || "";
  if (primarySymbol && !normalizedSymbols.includes(primarySymbol)) normalizedSymbols.unshift(primarySymbol);
  if (normalizedSymbols.length > MAX_SYMBOLS) return null;
  return { id, title, content, symbol: primarySymbol, symbols: normalizedSymbols, taskId: taskId.trim(), runId: runId.trim(),
    tags: tags.map(tag => tag.trim()), createdAt, updatedAt };
}
export function readResearchNotes(storage) {
  try {
    const target=safeStorage(storage); if(!target) return {items:[],readable:false,valid:false};
    const raw=target.getItem(RESEARCH_NOTES_KEY);
    if(raw===null) return {items:[],readable:true,valid:true};
    const parsed=JSON.parse(raw);
    if(!Array.isArray(parsed)||parsed.length>MAX_RESEARCH_NOTES) return {items:[],readable:true,valid:false};
    const items=parsed.map(normalizeNote);
    if(items.some(x=>!x)||new Set(items.map(x=>x.id)).size!==items.length) return {items:[],readable:true,valid:false};
    return {items,readable:true,valid:true};
  } catch { return {items:[],readable:false,valid:false}; }
}
export function writeResearchNotes(items, storage) {
  if(!Array.isArray(items)||items.length>MAX_RESEARCH_NOTES) return {ok:false,reason:"limit"};
  const normalized=items.map(normalizeNote);
  if(normalized.some(x=>!x)||new Set(normalized.map(x=>x.id)).size!==normalized.length) return {ok:false,reason:"invalid"};
  try {
    const target=safeStorage(storage); if(!target) return {ok:false,reason:"unavailable"};
    const existing=readResearchNotes(target);
    if(!existing.readable||!existing.valid) return {ok:false,reason:"invalid-storage"};
    target.setItem(RESEARCH_NOTES_KEY,JSON.stringify(normalized));
    const check=readResearchNotes(target);
    return check.readable&&check.valid&&JSON.stringify(check.items)===JSON.stringify(normalized)?{ok:true}:{ok:false,reason:"verify"};
  } catch { return {ok:false,reason:"unavailable"}; }
}
export function buildResearchNoteContextFromReport(report) {
  if (!report || typeof report !== "object") return null;
  const runId = String(report.runId || report.result?.run_id || "").trim();
  if (!runId) return null;
  const rawSymbols = Array.isArray(report.symbols) ? report.symbols : report.result?.request?.symbols;
  const symbols = Array.isArray(rawSymbols) ? [...new Set(rawSymbols.filter(value => typeof value === "string").map(value => value.trim()).filter(Boolean))] : [];
  const symbol = symbols[0] || "";
  return { symbol, symbols, taskId: String(report.taskId || "").trim(), runId };
}

export function filterResearchNotes(items, query="", symbol="") {
  const q=String(query||"").trim().toLowerCase(), s=String(symbol||"").trim().toLowerCase();
  return (Array.isArray(items)?items:[]).filter(n=>(!s||[n.symbol,...(n.symbols||[])].some(value=>value.toLowerCase().includes(s)))&&(!q||[n.title,n.content,n.symbol,...(n.symbols||[]),n.taskId,n.runId,...n.tags].join(" ").toLowerCase().includes(q))).sort((a,b)=>String(b.updatedAt).localeCompare(String(a.updatedAt)));
}
export function researchNoteEditState(items, editingId, originalSnapshot) {
  const current = (Array.isArray(items) ? items : []).find(item => item?.id === editingId);
  if (!current) return "missing";
  return JSON.stringify(current) === originalSnapshot ? "unchanged" : "changed";
}

export function bindResearchNotes({root=document, getContext=()=>({}), storage}={}) {
  const $=selector=>root.querySelector(selector.startsWith("#")?selector:"#"+selector), form=$("#research-note-form"); if(!form)return;
  let editingId="", editingSnapshot="";
  const feedback=(message,error=false)=>{const el=$("#research-note-feedback");el.textContent=message;el.dataset.state=error?"error":"ok";};
  const conflictActions=document.createElement("div");
  conflictActions.className="research-note-conflict-actions";
  conflictActions.hidden=true;
  const reloadButton=document.createElement("button");
  reloadButton.type="button";reloadButton.className="button button-secondary";reloadButton.textContent="重新载入最新版本";
  const saveAsNewButton=document.createElement("button");
  saveAsNewButton.type="button";saveAsNewButton.className="button button-secondary";saveAsNewButton.textContent="另存为新笔记";
  conflictActions.append(reloadButton,saveAsNewButton);
  $("#research-note-cancel").insertAdjacentElement("afterend",conflictActions);
  const hideConflictActions=()=>{conflictActions.hidden=true;};
  const showConflictActions=()=>{conflictActions.hidden=false;};
  const loadNoteIntoForm=note=>{
    $("#research-note-title").value=note.title;$("#research-note-content").value=note.content;
    $("#research-note-symbol").value=(note.symbols?.length?note.symbols:[note.symbol].filter(Boolean)).join(", ");$("#research-note-tags").value=note.tags.join(", ");
    $("#research-note-task").value=note.taskId;$("#research-note-run").value=note.runId;
  };
  reloadButton.addEventListener("click",()=>{
    const current=state();
    if(!current.readable||!current.valid){feedback("当前存储不可用，无法重新载入。",true);return;}
    const latest=current.items.find(item=>item.id===editingId);
    if(!latest){editingId="";editingSnapshot="";$("#research-note-cancel").hidden=true;hideConflictActions();feedback("该笔记已被其他标签页删除。当前草稿已保留为新笔记草稿，保存后会创建独立记录。",true);return;}
    loadNoteIntoForm(latest);editingSnapshot=JSON.stringify(latest);hideConflictActions();feedback("已载入其他标签页保存的最新版本。");
  });
  saveAsNewButton.addEventListener("click",()=>{
    editingId="";editingSnapshot="";$("#research-note-cancel").hidden=true;hideConflictActions();
    feedback("已切换为新笔记；当前草稿内容保留，提交后将创建独立记录。");
  });
  const state=()=>readResearchNotes(storage);
  const render=()=>{
    const host=$("#research-note-list"), result=state(); host.replaceChildren();
    if(!result.readable||!result.valid){const p=document.createElement("p");p.className="table-empty";p.textContent="浏览器存储不可读取或数据格式异常；为保护原始记录，当前未修改数据。";host.append(p);return;}
    const items=filterResearchNotes(result.items,$("#research-note-search").value,$("#research-note-symbol-filter").value);
    if(!items.length){const p=document.createElement("p");p.className="table-empty";p.textContent=result.items.length?"没有匹配的研究记录。":"尚无研究记录。";host.append(p);return;}
    items.forEach(note=>{
      const card=document.createElement("article");card.className="research-note-card";
      const header=document.createElement("header"), title=document.createElement("h3"), meta=document.createElement("span");
      title.textContent=note.title;meta.className="research-note-meta";meta.textContent=[(note.symbols?.length?note.symbols:[note.symbol].filter(Boolean)).join("、")||"未关联股票",note.taskId?"Task "+note.taskId:"未关联任务",note.runId?"Run "+note.runId:"",note.updatedAt||""].filter(Boolean).join(" · ");header.append(title,meta);card.append(header);
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
    if(editingId){
      const editState=researchNoteEditState(current.items,editingId,editingSnapshot);
      if(editState==="missing"){feedback("该笔记已被其他标签页删除，拒绝使用旧草稿重新创建。可重新载入状态或另存为新笔记。",true);showConflictActions();return;}
      if(editState==="changed"){feedback("该笔记已被其他标签页修改，拒绝用旧草稿覆盖。请重新载入最新版本或另存为新笔记。",true);showConflictActions();return;}
    }
    const context=getContext()||{};
    const tags=$("#research-note-tags").value.split(/[,，、]/).map(x=>x.trim()).filter(Boolean);
    if(tags.length>MAX_TAGS||tags.some(tag=>tag.length>40)){feedback("标签最多 12 个，每个标签不超过 40 个字符。",true);return;}
    const rawSymbols=$("#research-note-symbol").value.split(/[,，、\s]+/).map(value=>value.trim()).filter(Boolean);
    const fallbackSymbols=Array.isArray(context.symbols)?context.symbols:[context.symbol];
    const symbols=[...new Set((rawSymbols.length?rawSymbols:fallbackSymbols).filter(Boolean).map(value=>String(value).trim()))];
    if(symbols.length>MAX_SYMBOLS||symbols.some(value=>value.length>32)){feedback("关联股票最多 100 只，每个证券代码不超过 32 个字符。",true);return;}
    const note={id:editingId||("note-"+now+"-"+Math.random().toString(36).slice(2,8)),title,content,symbol:symbols[0]||"",symbols,taskId:$("#research-note-task").value.trim()||String(context.taskId||""),runId:$("#research-note-run").value.trim()||String(context.runId||""),tags,createdAt:existing?.createdAt||now,updatedAt:now};
    const next=existing?current.items.map(x=>x.id===editingId?note:x):[note,...current.items];
    if(!existing&&next.length>MAX_RESEARCH_NOTES){feedback("最多保存 200 条，请先整理或删除旧记录。",true);return;}
    const saved=writeResearchNotes(next,storage);if(!saved.ok){feedback("保存失败，原有记录未确认写入："+saved.reason,true);return;}
    editingId="";editingSnapshot="";form.reset();$("#research-note-cancel").hidden=true;hideConflictActions();feedback("研究记录已保存在当前浏览器，不会同步到服务端。");render();
  });
  $("#research-note-cancel").addEventListener("click",()=>{editingId="";editingSnapshot="";hideConflictActions();form.reset();const context=getContext()||{};$("#research-note-symbol").value=String(context.symbol||"");$("#research-note-task").value=String(context.taskId||"");$("#research-note-run").value=String(context.runId||"");$("#research-note-cancel").hidden=true;feedback("已取消编辑。");});
  $("#research-note-search").addEventListener("input",render);$("#research-note-symbol-filter").addEventListener("input",render);
  $("#research-note-list").addEventListener("click",event=>{
    const button=event.target.closest("[data-note-action]");if(!button)return;
    const current=state();if(!current.readable||!current.valid)return;
    const note=current.items.find(x=>x.id===button.dataset.noteId);if(!note)return;
    if(button.dataset.noteAction==="edit"){editingId=note.id;editingSnapshot=JSON.stringify(note);loadNoteIntoForm(note);hideConflictActions();$("#research-note-cancel").hidden=false;feedback("正在编辑记录。");form.scrollIntoView({behavior:"smooth",block:"center"});return;}
    if(!globalThis.confirm("确定删除这条研究记录？"))return;
    const saved=writeResearchNotes(current.items.filter(x=>x.id!==note.id),storage);feedback(saved.ok?"记录已删除。":"删除失败，未能确认存储结果。",!saved.ok);render();
  });
  const context=getContext()||{};
  $("#research-note-symbol").value=String(context.symbol||"");
  $("#research-note-task").value=String(context.taskId||"");
  $("#research-note-run").value=String(context.runId||"");
  render();
  return {
    render,
    createFromReport(report) {
      const linked = buildResearchNoteContextFromReport(report);
      if (!linked) return false;
      editingId = "";
      editingSnapshot = "";
      hideConflictActions();
      form.reset();
      $("#research-note-title").value = "Run " + linked.runId + " 研究记录";
      $("#research-note-symbol").value = linked.symbols.join(", ");
      $("#research-note-task").value = linked.taskId;
      $("#research-note-run").value = linked.runId;
      $("#research-note-cancel").hidden = true;
      feedback("已填入报告关联信息，请补充研究内容后保存。");
      form.scrollIntoView({ behavior: "smooth", block: "center" });
      $("#research-note-content").focus();
      return true;
    }
  };
}
