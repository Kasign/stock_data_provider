import { getMarketDragonTiger, getTradingCalendar } from "./api.js";
import { describeApiError, finiteNumberOrNull, formatCompact, formatDateTime, formatPercent, qualityLabel } from "./format.js";
const $=(s)=>document.querySelector(s);
const QUALITY=["valid","degraded","stale","invalid","unknown"];
function el(tag,cls="",text=""){const n=document.createElement(tag);if(cls)n.className=cls;if(text!==undefined&&text!==null)n.textContent=String(text);return n;}
function chinaParts(date=new Date()){const p=new Intl.DateTimeFormat("en-GB",{timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit"}).formatToParts(date);return Object.fromEntries(p.filter(x=>x.type!=="literal").map(x=>[x.type,x.value]));}
function dateKey(y,m,d){return String(y).padStart(4,"0")+"-"+String(m).padStart(2,"0")+"-"+String(d).padStart(2,"0");}
function monthRange(offset=0){const p=chinaParts(),d=new Date(Date.UTC(Number(p.year),Number(p.month)-1+offset,1)),y=d.getUTCFullYear(),m=d.getUTCMonth()+1;return {start:dateKey(y,m,1),end:dateKey(y,m,new Date(Date.UTC(y,m,0)).getUTCDate())};}
function badge(id,label,status="unknown"){const n=$("#"+id);if(!n)return;const q=QUALITY.includes(String(status).toLowerCase())?String(status).toLowerCase():"unknown";n.className="quality-pill quality-"+q;n.textContent=label||qualityLabel(q);}
function meta(id,value){const n=$("#"+id);if(n)n.textContent=value||"";}
function errorBox(id,message){const n=$("#"+id);if(n){n.hidden=!message;n.textContent=message||"";}}
function clearReasons(id){const n=$("#"+id);if(n){n.replaceChildren();n.hidden=true;}}
function messageRow(root,message,span){root.replaceChildren();const tr=document.createElement("tr"),td=el("td","table-empty",message);td.colSpan=span;tr.append(td);root.append(tr);}
function cell(row,value,cls=""){row.append(el("td",cls,value===null||value===undefined||value===""?"—":value));}
function amount(v){const n=finiteNumberOrNull(v);if(n===null)return "—";return (n>0?"+":n<0?"−":"")+formatCompact(Math.abs(n))+" 元";}
function direction(v){const n=finiteNumberOrNull(v);if(n===null)return "market-data-unknown";return n>0?"market-data-positive":n<0?"market-data-negative":"market-data-flat";}
function errorMessage(e){if(Number(e?.status)===404)return "当前 market-data 版本尚未提供此接口，请更新数据服务后再刷新。";if(Number(e?.status)===503)return "对应 Provider 未启用或未配置，请检查 market-data 数据源设置。";if([401,403].includes(Number(e?.status)))return "API Key 验证失败，请检查连接与密钥设置。";return describeApiError(e,"读取数据失败。");}
export function bindMarketActivity(getSettings,onSelectStock){
 const controllers={},generation={dragon:0,calendar:0};let dragonPage=1,dragonHasMore=false,dragonDate="",monthOffset=0;
 const settings=()=>typeof getSettings==="function"?getSettings():null;
 function abort(key){controllers[key]?.abort();delete controllers[key];generation[key]++;}
 function renderDragon(data,append){
  const root=$("#dragon-tiger-body"),items=Array.isArray(data?.items)?data.items.filter(x=>x&&typeof x==="object"):[];
  if(!append)root.replaceChildren();
  if(!items.length&&!append)messageRow(root,data?.has_data===false?"源端当前未确认有可展示的 A 股龙虎榜记录；盘后数据可能尚未发布，也可能有非 A 股记录被过滤。":"接口未返回龙虎榜明细。",6);
  items.forEach(item=>{
   const tr=document.createElement("tr"),stock=el("td","activity-stock-cell");
   const symbol=String(item.symbol||item.code||"").trim();
   stock.append(el("strong","market-table-primary",item.name||"股票名称未报告"),el("small","market-table-secondary",symbol||"代码未报告"));
   if(symbol){
    const button=el("button","activity-stock-link","查看行情");button.type="button";button.dataset.dragonSymbol=symbol;button.dataset.dragonName=item.name||symbol;stock.append(button);
   }
   tr.append(stock);
   cell(tr,formatPercent(item.change_percent),direction(item.change_percent));cell(tr,amount(item.net_buy_amount_cny),direction(item.net_buy_amount_cny));cell(tr,amount(item.deal_amount_cny));
   const reason=el("td","dragon-tiger-reason",item.reason||item.explanation||"—");if(item.explanation)reason.title=item.explanation;tr.append(reason);
   const q=QUALITY.includes(String(item.quality_status||"").toLowerCase())?String(item.quality_status).toLowerCase():"unknown",td=el("td");td.append(el("span","quality-pill quality-"+q,qualityLabel(q)));tr.append(td);root.append(tr);
  });
  badge("dragon-tiger-quality",qualityLabel(data?.quality_status),data?.quality_status);
  meta("dragon-tiger-meta",["交易日 "+(data?.trade_date||dragonDate||"中国时区当日（默认）"),"第 "+(data?.page??dragonPage)+" 页","本页 "+(data?.returned??items.length)+" 条","源端 "+(data?.source_returned??"未报告")+" 条","过滤非 A 股 "+(data?.excluded_non_a_share_count??"未报告")+" 条","来源 "+(data?.source||"未报告"),"获取 "+formatDateTime(data?.fetched_at)].join(" · "));
  const rr=$("#dragon-tiger-reasons");rr.replaceChildren();const rs=Array.isArray(data?.quality_reasons)?data.quality_reasons:[];rr.hidden=!rs.length;rs.forEach(x=>rr.append(el("li","",x)));
  dragonPage=Number(data?.page)||1;dragonHasMore=Boolean(data?.has_more);$("#dragon-tiger-next").hidden=!dragonHasMore;$("#dragon-tiger-next").disabled=!dragonHasMore;$("#dragon-tiger-next").textContent=dragonHasMore?"加载下一页":"没有更多页";
 }
 function invalidateDragonFilter(){
  abort("dragon");dragonPage=1;dragonHasMore=false;
  errorBox("dragon-tiger-error","");badge("dragon-tiger-quality","条件已变更","unknown");
  meta("dragon-tiger-meta","日期筛选已变更，请重新加载龙虎榜。");clearReasons("dragon-tiger-reasons");
  messageRow($("#dragon-tiger-body"),"交易日期已变化，请点击“读取龙虎榜”加载新条件。",6);
  const next=$("#dragon-tiger-next");next.hidden=true;next.disabled=true;next.textContent="加载下一页";
 }
 async function loadDragon({append=false}={}){
  abort("dragon");const seq=generation.dragon,cfg=settings();
  if(!cfg?.apiKey){
   messageRow($("#dragon-tiger-body"),"请先配置行情 API Key。",6);
   badge("dragon-tiger-quality","需配置 Key");
   errorBox("dragon-tiger-error","");
   meta("dragon-tiger-meta","");
   clearReasons("dragon-tiger-reasons");
   dragonHasMore=false;$("#dragon-tiger-next").hidden=true;$("#dragon-tiger-next").disabled=true;
   return;
  }
  if(!append){
   dragonPage=1;dragonHasMore=false;dragonDate=$("#dragon-tiger-date").value;
   messageRow($("#dragon-tiger-body"),"正在读取龙虎榜…",6);
   badge("dragon-tiger-quality","加载中","unknown");meta("dragon-tiger-meta","");
   clearReasons("dragon-tiger-reasons");$("#dragon-tiger-next").hidden=true;
  } else {$("#dragon-tiger-next").disabled=true;$("#dragon-tiger-next").textContent="正在加载…";}
  errorBox("dragon-tiger-error","");const ctl=new AbortController();controllers.dragon=ctl;
  try{const data=await getMarketDragonTiger(cfg,{date:dragonDate,page:append?dragonPage+1:1,pageSize:50,signal:ctl.signal});if(ctl.signal.aborted||seq!==generation.dragon)return;renderDragon(data,append);}
  catch(e){
   if(ctl.signal.aborted||seq!==generation.dragon)return;
   errorBox("dragon-tiger-error",errorMessage(e));
   if(!append){
    badge("dragon-tiger-quality","读取失败","invalid");meta("dragon-tiger-meta","");
    clearReasons("dragon-tiger-reasons");
    messageRow($("#dragon-tiger-body"),"龙虎榜暂不可用，请查看错误说明后重试。",6);
   }
  }
  finally{if(generation.dragon===seq){delete controllers.dragon;$("#dragon-tiger-next").disabled=!dragonHasMore;$("#dragon-tiger-next").textContent=dragonHasMore?"加载下一页":"没有更多页";}}
 }
 function setMonth(offset){monthOffset=offset;const range=monthRange(offset);$("#calendar-start-date").value=range.start;$("#calendar-end-date").value=range.end;meta("calendar-month-label",range.start.slice(0,7));}
 function calendarTimestamp(value){
  const date=String(value??"");
  if(!/^\d{4}-\d{2}-\d{2}$/.test(date))return NaN;
  const timestamp=Date.parse(date+"T00:00:00Z");
  return Number.isFinite(timestamp)&&new Date(timestamp).toISOString().slice(0,10)===date?timestamp:NaN;
}
function renderCalendarDay(root,date,item){
  if(!item){
   const day=el("div","trading-calendar-day is-missing-day");
   day.append(el("strong","",date.slice(-2)),el("span","","未返回"));
   day.title=date+" · 接口未返回该日期的交易日状态";root.append(day);return;
  }
  const status=item.is_trading_day===true?"is-trading-day":item.is_trading_day===false?"is-closed-day":"is-unknown-day";
  const label=item.is_trading_day===true?"交易日":item.is_trading_day===false?"休市":"未确认";
  const day=el("div","trading-calendar-day "+status);
  day.append(el("strong","",String(item.date||date).slice(-2)),el("span","",label));
  day.title=(item.date||date)+" · "+(item.is_trading_day===true?"服务端确认交易日":item.is_trading_day===false?"服务端确认非交易日":"服务端未报告交易状态");
  root.append(day);
}
function renderCalendar(data){
  const root=$("#trading-calendar-grid"),items=Array.isArray(data?.items)?data.items.filter(x=>x&&typeof x==="object"):[];root.replaceChildren();
  const start=String(data?.start_date||$("#calendar-start-date")?.value||items[0]?.date||"");
  const end=String(data?.end_date||$("#calendar-end-date")?.value||items[items.length-1]?.date||start);
  const startTime=calendarTimestamp(start),endTime=calendarTimestamp(end);
  const validRange=Number.isFinite(startTime)&&Number.isFinite(endTime)&&endTime>=startTime&&(endTime-startTime)/86400000<=366;
  const expectedDays=validRange?Math.round((endTime-startTime)/86400000)+1:null;
  let missingDates=0;
  if(!items.length)root.append(el("p","market-empty-state","接口未返回交易日历明细；不会通过周一至周五自行推断交易日。"));
  else if(!validRange)root.append(el("p","market-empty-state","接口返回的日期范围无效，无法可靠对齐交易日历。"));
  else{
   const weekday=new Date(startTime).getUTCDay();
   for(let i=0;i<weekday;i++)root.append(el("span","trading-calendar-padding",""));
   const byDate=new Map();
   items.forEach(item=>{const date=String(item.date||"");if(Number.isFinite(calendarTimestamp(date))&&date>=start&&date<=end)byDate.set(date,item);});
   for(let timestamp=startTime;timestamp<=endTime;timestamp+=86400000){
    const date=new Date(timestamp).toISOString().slice(0,10),item=byDate.get(date);
    if(!item)missingDates++;
    renderCalendarDay(root,date,item);
   }
  }
  badge("trading-calendar-quality",qualityLabel(data?.quality_status),data?.quality_status);
  meta("trading-calendar-meta",["请求 "+(data?.start_date||start||"未报告")+" 至 "+(data?.end_date||end||"未报告"),"覆盖 "+items.length+" / "+(data?.total_days??"未报告")+" 天","交易日 "+(data?.trading_days??"未报告")+" 天","来源 "+(data?.source||"未报告"),"获取 "+formatDateTime(data?.fetched_at)].join(" · "));
  const rr=$("#trading-calendar-reasons");rr.replaceChildren();const rs=Array.isArray(data?.quality_reasons)?data.quality_reasons:[];rr.hidden=!rs.length;rs.forEach(x=>rr.append(el("li","",x)));
  const totalDays=data?.total_days;
  const hasReportedTotal=totalDays!==null&&totalDays!==undefined&&totalDays!==""&&Number.isFinite(Number(totalDays));
  const reportedMismatch=hasReportedTotal&&Number(totalDays)!==items.length;
  const rangeMismatch=expectedDays!==null&&expectedDays!==items.length;
  if(missingDates>0||reportedMismatch||rangeMismatch||(!validRange&&items.length))root.prepend(el("p","calendar-coverage-warning","返回日期数与请求范围或源端报告天数不一致；缺失日期会标示为“未返回”，请勿将不完整日历用于交易日判断。"));
 }
 function syncCalendarMonthNavigation(){
  const start=String($("#calendar-start-date").value||""),end=String($("#calendar-end-date").value||"");
  const startTime=calendarTimestamp(start),endTime=calendarTimestamp(end);
  if(Number.isFinite(startTime)){
   const date=new Date(startTime),current=chinaParts();
   const selectedYear=date.getUTCFullYear(),selectedMonth=date.getUTCMonth();
   monthOffset=selectedYear*12+selectedMonth-(Number(current.year)*12+Number(current.month)-1);
  }
  const label=Number.isFinite(startTime)&&Number.isFinite(endTime)&&start<=end
   ?start.slice(0,7)+(start.slice(0,7)===end.slice(0,7)?"":" — "+end.slice(0,7))
   :"日期范围待修正";
  meta("calendar-month-label",label);
 }
 function invalidateCalendarFilter(){
  abort("calendar");syncCalendarMonthNavigation();
  errorBox("trading-calendar-error","");badge("trading-calendar-quality","条件已变更","unknown");
  meta("trading-calendar-meta","日期筛选已变更，请重新加载交易日历。");
  clearReasons("trading-calendar-reasons");
  $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state","日期范围已变化，请点击“读取交易日历”加载新条件。"));
 }
 async function loadCalendar(){
  abort("calendar");const seq=generation.calendar,cfg=settings(),start=$("#calendar-start-date").value,end=$("#calendar-end-date").value;
  if(!/^\d{4}-\d{2}-\d{2}$/.test(start)||!/^\d{4}-\d{2}-\d{2}$/.test(end)||start>end){
   const message="请选择有效日期，并确保开始日期不晚于结束日期。";
   errorBox("trading-calendar-error",message);badge("trading-calendar-quality","日期无效","invalid");
   meta("trading-calendar-meta","");clearReasons("trading-calendar-reasons");
   $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state",message));return;
  }
  const a=Date.parse(start+"T00:00:00Z"),b=Date.parse(end+"T00:00:00Z");
  if(!Number.isFinite(a)||!Number.isFinite(b)||b-a>366*86400000){
   const message="日期区间最多 367 个自然日，请缩小范围。";
   errorBox("trading-calendar-error",message);badge("trading-calendar-quality","日期范围无效","invalid");
   meta("trading-calendar-meta","");clearReasons("trading-calendar-reasons");
   $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state",message));return;
  }
  if(!cfg?.apiKey){
   const message="请先在连接与密钥设置中配置行情 API Key。";
   errorBox("trading-calendar-error",message);badge("trading-calendar-quality","需配置 Key");
   meta("trading-calendar-meta","数据未读取 · 需要 API Key");clearReasons("trading-calendar-reasons");
   $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state",message));return;
  }
  errorBox("trading-calendar-error","");
  meta("calendar-month-label",start.slice(0,7)+(start.slice(0,7)===end.slice(0,7)?"":" — "+end.slice(0,7)));
  badge("trading-calendar-quality","加载中","unknown");
  meta("trading-calendar-meta","请求 "+start+" 至 "+end+" · 正在读取");
  clearReasons("trading-calendar-reasons");
  $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state","正在读取交易日历…"));const ctl=new AbortController();controllers.calendar=ctl;
  try{const data=await getTradingCalendar(cfg,{start,end,signal:ctl.signal});if(ctl.signal.aborted||seq!==generation.calendar)return;renderCalendar(data);}
  catch(e){
   if(ctl.signal.aborted||seq!==generation.calendar)return;
   badge("trading-calendar-quality","读取失败","invalid");errorBox("trading-calendar-error",errorMessage(e));
   meta("trading-calendar-meta","请求 "+start+" 至 "+end+" · 本次未返回数据");
   clearReasons("trading-calendar-reasons");
   $("#trading-calendar-grid").replaceChildren(el("p","market-empty-state","交易日历暂不可用，请查看错误说明后重试。"));
  }
  finally{if(generation.calendar===seq)delete controllers.calendar;}
 }
 const today=chinaParts();$("#dragon-tiger-date").value=dateKey(today.year,today.month,today.day);setMonth(0);
 $("#load-dragon-tiger").addEventListener("click",()=>void loadDragon());$("#dragon-tiger-next").addEventListener("click",()=>{if(dragonHasMore)void loadDragon({append:true});});
 $("#dragon-tiger-date").addEventListener("change",invalidateDragonFilter);
 $("#dragon-tiger-date").addEventListener("keydown",e=>{if(e.key==="Enter"){e.preventDefault();void loadDragon();}});
 $("#load-trading-calendar").addEventListener("click",()=>void loadCalendar());
 $("#calendar-start-date").addEventListener("change",invalidateCalendarFilter);
 $("#calendar-end-date").addEventListener("change",invalidateCalendarFilter);
 $("#calendar-previous-month").addEventListener("click",()=>{setMonth(monthOffset-1);void loadCalendar();});
 $("#calendar-current-month").addEventListener("click",()=>{setMonth(0);void loadCalendar();});
 $("#calendar-next-month").addEventListener("click",()=>{setMonth(monthOffset+1);void loadCalendar();});
 $("#dragon-tiger-body").addEventListener("click",event=>{const b=event.target.closest("button[data-dragon-symbol]");if(b&&b.dataset.dragonSymbol&&typeof onSelectStock==="function")onSelectStock({symbol:b.dataset.dragonSymbol,name:b.dataset.dragonName||b.dataset.dragonSymbol});});
 return {refresh:()=>Promise.all([loadDragon(),loadCalendar()])};
}
