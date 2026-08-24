const state = { showcases: [], items: [], selected: null, detail: null };
const labels = {
  state: {reported:"已登记",assessed:"已分级",plan_pending:"待审批",approved:"待执行",executing:"处置中",verifying:"复测中",closed:"已归档"},
  severity: {low:"低",medium:"中",high:"高",critical:"紧急"},
  metric: {temperature:"温度",humidity:"相对湿度"},
  source: {sensor:"传感器",manual:"人工"}
};

const $ = (selector, root=document) => root.querySelector(selector);
const escapeHTML = value => String(value ?? "").replace(/[&<>'"]/g, char => ({"&":"&amp;","<":"&lt;",">":"&gt;","'":"&#39;",'"':"&quot;"}[char]));
const formatTime = value => value ? new Intl.DateTimeFormat("zh-CN", {month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false}).format(new Date(value)) : "-";
const localInput = date => { const d = new Date(date); d.setMinutes(d.getMinutes()-d.getTimezoneOffset()); return d.toISOString().slice(0,16); };
const idemKey = prefix => `${prefix}-${crypto.randomUUID()}`;

async function api(path, options={}) {
  const headers = {Accept:"application/json", ...(options.headers || {})};
  if (options.body) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {...options, headers});
  let payload;
  try { payload = await response.json(); } catch { payload = {error:{message:"服务返回了无法解析的响应"}}; }
  if (!response.ok) {
    const error = new Error(payload.error?.message || `请求失败 (${response.status})`);
    error.status = response.status; error.fields = payload.error?.fields;
    throw error;
  }
  return payload;
}

function toast(message, error=false) {
  const node = $("#toast"); node.textContent = message; node.className = `toast${error ? " error" : ""}`;
  clearTimeout(toast.timer); toast.timer = setTimeout(() => node.classList.add("hidden"), 4200);
}

async function loadShowcases() {
  const response = await api("/api/showcases"); state.showcases = response.data;
  const options = response.data.map(item => `<option value="${escapeHTML(item.id)}">${escapeHTML(item.name)} · ${escapeHTML(item.gallery_zone)}</option>`).join("");
  $("#showcase-filter").insertAdjacentHTML("beforeend", options);
  $("#create-form [name=showcase_id]").innerHTML = options;
}

async function loadQueue(preferredID) {
  const params = new URLSearchParams();
  if ($("#state-filter").value) params.set("state", $("#state-filter").value);
  if ($("#showcase-filter").value) params.set("showcase_id", $("#showcase-filter").value);
  const response = await api(`/api/incidents?${params}`); state.items = response.data;
  $("#queue-summary").textContent = `${response.meta.count} 项 · 按最近更新排序`;
  renderQueue();
  const routeID = location.pathname.startsWith("/incidents/") ? location.pathname.split("/")[2] : "";
  const selectID = preferredID || routeID || state.selected;
  if (selectID) await selectIncident(selectID, false);
}

function renderQueue() {
  const list = $("#incident-list");
  if (!state.items.length) { list.innerHTML = `<div class="empty-list">当前筛选下没有异常</div>`; return; }
  list.innerHTML = state.items.map(({incident,showcase}) => `<button class="incident-item ${incident.id===state.selected?"selected":""}" data-id="${escapeHTML(incident.id)}">
    <div class="item-top"><span class="badge severity-${incident.severity}">${labels.severity[incident.severity]}优先级</span><span class="metric-value">${escapeHTML(incident.observed_value)} ${escapeHTML(incident.unit)}</span></div>
    <div class="item-title">${escapeHTML(showcase.name)} · ${labels.metric[incident.metric]}</div><div class="item-description">${escapeHTML(incident.description || "未填写描述")}</div>
    <div class="item-bottom"><span>${escapeHTML(showcase.gallery_zone)} · ${formatTime(incident.detected_at)}</span><span>${labels.state[incident.state]} · r${incident.revision}</span></div></button>`).join("");
  list.querySelectorAll("[data-id]").forEach(node => node.addEventListener("click", () => selectIncident(node.dataset.id, true)));
}

async function selectIncident(id, push) {
  try {
    const response = await api(`/api/incidents/${encodeURIComponent(id)}`); state.selected = id; state.detail = response.data;
    if (push && location.pathname !== `/incidents/${id}`) history.pushState({}, "", `/incidents/${id}`);
    $("#empty-state").classList.add("hidden"); $("#detail-content").classList.remove("hidden"); renderQueue(); renderDetail();
  } catch (error) { showError(error); }
}

function renderDetail() {
  const {incident,showcase,assessment,plan,actions,verifications,audit,archive,reports=[]} = state.detail;
  $("#detail-kicker").textContent = `${showcase.gallery_zone} / ${showcase.id} / ${labels.source[incident.source]}发现`;
  $("#detail-title").textContent = `${showcase.name} · ${labels.metric[incident.metric]}偏差`;
  $("#detail-description").textContent = incident.description || "未填写异常描述";
  $("#detail-severity").className = `badge severity-${incident.severity}`; $("#detail-severity").textContent = `${labels.severity[incident.severity]}优先级`;
  $("#detail-state").textContent = labels.state[incident.state]; $("#detail-revision").textContent = `revision ${incident.revision}`;
  const scope={showcase_only:"当前展柜",zone_watch:"展区联动观察",collection_emergency:"藏品应急响应"};
  $("#baseline").innerHTML = `<div class="baseline-cell"><span>异常读数 / 偏差</span><strong>${escapeHTML(incident.observed_value)} ${escapeHTML(incident.unit)} / ${assessment?.deviation ?? "-"}</strong></div><div class="baseline-cell"><span>温湿度目标</span><strong>${showcase.target_temperature_min}–${showcase.target_temperature_max} °C · ${showcase.target_humidity_min}–${showcase.target_humidity_max} %RH</strong></div><div class="baseline-cell"><span>影响范围</span><strong>${scope[assessment?.scope] || "待评估"}</strong></div><div class="baseline-cell"><span>响应时限</span><strong>${formatTime(assessment?.response_due_at)}</strong></div>`;
  renderActionZone(incident, plan, archive); renderRecords(plan, actions, verifications, reports); renderAudit(audit);
}

function renderActionZone(incident, plan, archive) {
  const zone = $("#action-zone"); const revision = incident.revision;
  if (incident.state === "assessed") zone.innerHTML = `<div class="workflow-box"><div class="workflow-copy"><h3>编制处置方案</h3><p>明确责任人、完成时限和有序步骤，提交值班主管审批。</p></div><form class="inline-form" id="plan-form"><label><span>责任人</span><input name="owner" required></label><label><span>完成时限</span><input name="due_at" type="datetime-local" value="${localInput(Date.now()+2*3600000)}" required></label><label class="wide"><span>处置步骤（每行一项）</span><textarea name="steps" rows="3" required></textarea></label><label class="wide"><span>风险提示</span><input name="risk_notes"></label><label><span>提交人</span><input name="actor" required></label><div class="submit-row"><button class="primary" type="submit">提交审批</button></div></form></div>`;
  else if (incident.state === "plan_pending") zone.innerHTML = `<div class="workflow-box"><div class="workflow-copy"><h3>主管审批</h3><p>责任人：${escapeHTML(plan?.owner)}<br>时限：${formatTime(plan?.due_at)}</p></div><form class="inline-form" id="approval-form"><label><span>审批人</span><input name="approver" required></label><label><span>审批意见</span><input name="comment"></label><div class="decision-row"><button class="secondary" name="decision" value="reject" type="submit">退回修订</button><button class="primary" name="decision" value="approve" type="submit">批准执行</button></div></form></div>`;
  else if (["approved","executing"].includes(incident.state)) zone.innerHTML = `<div class="workflow-box"><div class="workflow-copy"><h3>记录现场动作</h3><p>${incident.state === "approved" ? "方案已批准，可以开始现场处置。" : "继续记录动作，完成后可直接录入复测。"}</p></div><form class="inline-form" id="action-form"><label><span>操作者</span><input name="operator" required></label><label><span>动作时间</span><input name="performed_at" type="datetime-local" value="${localInput(Date.now())}" required></label><label class="wide"><span>动作内容</span><textarea name="description" rows="2" required></textarea></label><label class="wide"><span>照片或文本证据引用</span><input name="evidence_ref" placeholder="馆内影像编号或文件引用"></label><div class="submit-row">${incident.state === "executing" ? '<button class="secondary" id="open-verification" type="button">录入复测</button>' : ""}<button class="primary" type="submit">保存动作</button></div></form></div>`;
  else if (incident.state === "verifying") zone.innerHTML = verificationForm("再次复测", "需要连续两次温湿度均在目标区间，事件才会安全关闭。", revision);
  else if (incident.state === "closed") zone.innerHTML = `<div class="workflow-box"><div class="workflow-copy"><h3>事件已安全关闭</h3><p>${escapeHTML(archive?.resolution_summary || "恢复条件已满足")}</p></div><div><span class="target-ok">档案编号 ${escapeHTML(archive?.id)}</span></div></div>`;
  else zone.innerHTML = "";
  const actionForm = $("#action-form");
  if (actionForm && plan?.steps?.length) {
    const label = document.createElement("label");
    label.innerHTML = "<span>方案步骤</span>";
    const select = document.createElement("select"); select.name = "step_order"; select.required = true;
    plan.steps.forEach(step => { const option = document.createElement("option"); option.value = step.order; option.textContent = step.order + ". " + step.instruction; select.append(option); });
    label.append(select); actionForm.prepend(label);
  }
  bindActionForms(revision);
}

function verificationForm(title="录入复测", copy="系统会同时检查温度和相对湿度目标区间。") {
  return `<div class="workflow-box"><div class="workflow-copy"><h3>${title}</h3><p>${copy}</p></div><form class="inline-form" id="verification-form"><label><span>温度 °C</span><input name="temperature" type="number" step="0.1" required></label><label><span>相对湿度 %RH</span><input name="humidity" type="number" step="0.1" required></label><label><span>仪器编号</span><input name="instrument_id" required></label><label><span>测量时间</span><input name="measured_at" type="datetime-local" value="${localInput(Date.now())}" required></label><label><span>操作员</span><input name="operator" required></label><label><span>备注</span><input name="comment"></label><div class="submit-row"><button class="primary" type="submit">保存并自动判定</button></div></form></div>`;
}

function values(form) { return Object.fromEntries(new FormData(form).entries()); }
function iso(value) { return new Date(value).toISOString(); }
async function submitCommand(path, payload, prefix) {
  try { const response = await api(path,{method:"POST",headers:{"Idempotency-Key":idemKey(prefix)},body:JSON.stringify(payload)}); state.detail=response.data; renderDetail(); await loadQueue(state.selected); toast("操作已保存"); }
  catch(error) { showError(error); }
}

function bindActionForms(revision) {
  $("#plan-form")?.addEventListener("submit", event => { event.preventDefault(); const v=values(event.currentTarget); submitCommand(`/api/incidents/${state.selected}/plans`,{expected_revision:revision,owner:v.owner,due_at:iso(v.due_at),steps:v.steps.split("\n").filter(Boolean).map((instruction,index)=>({order:index+1,instruction})),risk_notes:v.risk_notes,actor:v.actor},"plan"); });
  $("#approval-form")?.addEventListener("submit", event => { event.preventDefault(); const v=values(event.currentTarget); const decision=event.submitter.value; submitCommand(`/api/incidents/${state.selected}/plans/approval`,{expected_revision:revision,approver:v.approver,approved:decision==="approve",comment:v.comment},"approval"); });
  $("#action-form")?.addEventListener("submit", event => { event.preventDefault(); const v=values(event.currentTarget); submitCommand(`/api/incidents/${state.selected}/actions`,{expected_revision:revision,step_order:Number(v.step_order),performed_at:iso(v.performed_at),operator:v.operator,description:v.description,evidence_ref:v.evidence_ref},"action"); });
  $("#open-verification")?.addEventListener("click", () => { $("#action-zone").innerHTML=verificationForm(); bindActionForms(revision); });
  $("#verification-form")?.addEventListener("submit", event => { event.preventDefault(); const v=values(event.currentTarget); submitCommand(`/api/incidents/${state.selected}/verifications`,{expected_revision:revision,measured_at:iso(v.measured_at),temperature:Number(v.temperature),humidity:Number(v.humidity),instrument_id:v.instrument_id,comment:v.comment,operator:v.operator},"verify"); });
}

function renderRecords(plan, actions, verifications, reports=[]) {
  const records=[];
  if(plan) records.push(`<article class="record-entry"><header><strong>处置方案 · ${plan.status==="approved"?"已批准":plan.status==="rejected"?"已退回":"待审批"}</strong><small>revision ${plan.revision}</small></header><p>${plan.steps.map(step=>`${step.order}. ${escapeHTML(step.instruction)}`).join("<br>")}</p><small>责任人 ${escapeHTML(plan.owner)} · 截止 ${formatTime(plan.due_at)}</small></article>`);
  actions.forEach(action=>records.push(`<article class="record-entry"><header><strong>现场动作 · ${escapeHTML(action.operator)}</strong><small>${formatTime(action.performed_at)}</small></header><p>${escapeHTML(action.description)}</p>${action.evidence_ref?`<small>证据：${escapeHTML(action.evidence_ref)}</small>`:""}</article>`));
  verifications.forEach(item=>records.push(`<article class="record-entry"><header><strong>复测 · ${escapeHTML(item.operator)}</strong><small>${formatTime(item.measured_at)}</small></header><p>${item.temperature} °C / ${item.humidity} %RH · <span class="${item.within_target?"target-ok":"target-bad"}">${item.within_target?"位于目标区间":"未恢复"}</span></p><small>仪器 ${escapeHTML(item.instrument_id)}${item.comment?` · ${escapeHTML(item.comment)}`:""}</small></article>`));
  reports.slice(1).forEach(item=>records.push("<article class=\"record-entry\"><header><strong>补报 · "+escapeHTML(item.reporter)+"</strong><small>"+formatTime(item.detected_at)+"</small></header><p>"+escapeHTML(item.description||"未填写描述")+"</p></article>"));
  $("#record-list").innerHTML=records.join("")||'<div class="empty-list">尚无处置记录</div>'; $("#record-count").textContent=`${records.length} 项`;
}
function renderAudit(audit) { $("#audit-list").innerHTML=[...audit].reverse().map(item=>`<li><strong>${escapeHTML(item.summary)}</strong><span>${escapeHTML(item.actor)} · ${formatTime(item.at)} · revision ${item.revision}</span></li>`).join(""); }

async function loadArchives(query="") {
  try { const response=await api(`/api/archives?q=${encodeURIComponent(query)}`); const list=$("#archive-list"); if(!response.data.length){list.innerHTML='<div class="empty-list">没有匹配的处置档案</div>';return;} list.innerHTML='<div class="archive-row header"><span>事件 / 展柜</span><span>关闭时间</span><span>责任人</span><span>记录数</span><span>处置结论</span></div>'+response.data.map(item=>`<div class="archive-row"><div class="archive-title"><strong>${escapeHTML(item.showcase_name)}</strong><small>${escapeHTML(item.incident_id)}</small></div><span>${formatTime(item.closed_at)}</span><span>${escapeHTML(item.owner)}</span><span>${item.action_count} 动作 / ${item.verification_count} 复测</span><span>${escapeHTML(item.resolution_summary)}</span></div>`).join(""); } catch(error){showError(error);}
}
function showError(error) { const fieldText=error.fields?`：${Object.values(error.fields).join("；")}`:""; toast(`${error.message}${fieldText}${error.status===409?"。数据可能已更新，请刷新后重试。":""}`,true); if(error.status===409&&state.selected) selectIncident(state.selected,false); }

function bindShell() {
  $("#state-filter").addEventListener("change",()=>loadQueue()); $("#showcase-filter").addEventListener("change",()=>loadQueue());
  $("#open-create").addEventListener("click",()=>{ $("#create-form [name=detected_at]").value=localInput(Date.now()); $("#create-dialog").showModal(); });
  document.querySelectorAll(".close-dialog").forEach(button=>button.addEventListener("click",()=>$("#create-dialog").close()));
  $("#create-form").addEventListener("submit",async event=>{event.preventDefault();const v=values(event.currentTarget);try{const response=await api("/api/incidents",{method:"POST",headers:{"Idempotency-Key":idemKey("create")},body:JSON.stringify({...v,observed_value:Number(v.observed_value),detected_at:iso(v.detected_at)})});$("#create-dialog").close();event.currentTarget.reset();await loadQueue(response.data.incident.id);toast(response.meta.created?"异常已登记并完成分级":"已找到相同指纹的在办异常");}catch(error){showError(error);}});
  document.querySelectorAll(".nav-button").forEach(button=>button.addEventListener("click",()=>{document.querySelectorAll(".nav-button").forEach(n=>n.classList.toggle("active",n===button));$("#queue-view").classList.toggle("hidden",button.dataset.view!=="queue");$("#archives-view").classList.toggle("hidden",button.dataset.view!=="archives");if(button.dataset.view==="archives"){history.pushState({},"","/archives");loadArchives();}else{history.pushState({},"","/");}}));
  $("#archive-search").addEventListener("submit",event=>{event.preventDefault();loadArchives($("#archive-query").value);});
  addEventListener("popstate",()=>location.reload());
}

document.addEventListener("DOMContentLoaded",async()=>{bindShell();try{await loadShowcases();if(location.pathname==="/archives"){$('[data-view="archives"]').click();}else await loadQueue();}catch(error){showError(error);}});
