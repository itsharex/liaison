// Explicit staging acceptance; synthetic SELECT or review-only draft, no writes.
const {chromium,request}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 assert(process.env.E2E_BASE_URL&&process.env.E2E_TOKEN_FILE,'Explicit staging URL and token file required');
 const {token}=JSON.parse(fs.readFileSync(process.env.E2E_TOKEN_FILE,'utf8'));
 const baseURL=process.env.E2E_BASE_URL;
 const handoff=process.env.E2E_EDITOR_HANDOFF==='1';
 const review=process.env.E2E_REVIEW_DRAFT==='1'||handoff;
 const api=await request.newContext({baseURL,ignoreHTTPSErrors:true,timeout:180000,extraHTTPHeaders:{Authorization:`Bearer ${token}`}});
 const call=async(method,path,data)=>{const r=await api.fetch(path,{method,data}),b=await r.json();assert(r.ok()&&[200,201].includes(b.code),`${method}: HTTP ${r.status()}, code ${b.code}`);return b.data;};
 const sessions=new Set(),agents=new Set();let browser,restore;
 try{
  const proxy=(await call('GET','/api/v1/proxies?page_size=200')).proxies.find(p=>p.name==='Agent Demo mysql');
  assert(proxy&&!proxy.expose_public_port,'Private MySQL demo required');
  if(proxy.status==='stopped'){restore=proxy;await call('PUT',`/api/v1/proxies/${proxy.id}`,{status:'running'});}
  const target=await call('GET',`/api/v1/webdata/proxies/${proxy.id}`),credential=target.credentials.find(c=>c.saved);assert(credential);
  browser=await chromium.launch();const context=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000}});
  await context.addInitScript(token=>{localStorage.setItem('token',token);localStorage.setItem('liaison-locale','zh-CN');localStorage.setItem('liaison-theme-preference','dark');},token);
  const page=await context.newPage(),errors=[],turns=[];let executions=0;
  page.on('pageerror',e=>errors.push(e.message));
  page.on('request',r=>{if(r.method()==='POST'){if(r.url().endsWith('/turns'))turns.push(r.postDataJSON());if(r.url().endsWith('/execute'))executions++;}});
  const captures=new Set();
  page.on('response',r=>{if(r.request().method()!=='POST'||!r.ok())return;
   const capture=(async()=>{if(r.url().endsWith('/session')){const b=await r.json();if(b.data?.token)sessions.add(b.data.token);}if(r.url().endsWith('/agent/sessions')){const b=await r.json();if(b.data?.session?.id)agents.add(b.data.session.id);}})();
   captures.add(capture);capture.finally(()=>captures.delete(capture));
  });
  const button=name=>page.getByRole('button',{name,exact:true});
  try{
   await page.goto(`${baseURL}/webdata/${proxy.id}/connections/${credential.id}?from=%2Fproxy%3Fcategory%3Ddatabase`);
   await page.locator('.webdata-code-editor textarea').fill(review?'DELETE FROM liaison_review_example;':"SELECT 42 AS total, 'synthetic-secret' AS password");
   if(!review)await button('执行').click();
   await button(review?'审阅草稿':'分析结果').click();const dialog=page.getByRole('dialog');await dialog.waitFor();
   const preview=await dialog.locator('textarea').inputValue();
   if(review)assert.equal(preview,'DELETE FROM liaison_review_example;');
   else assert(preview.includes('42')&&preview.includes('[redacted]')&&!preview.includes('synthetic-secret'));
   await page.screenshot({path:review?'/tmp/draft-review-live-preview.png':'/tmp/result-share-live-preview.png'});
   await button('放入 Agent 草稿').click();await dialog.waitFor({state:'hidden'});
   await page.waitForFunction(needle=>document.querySelector('.agent-workspace textarea')?.value.includes(needle),review?'liaison_review_example':'42');
   assert.equal(turns.length,0,'Adding a draft must not send it');assert.equal(executions,review?0:1);
   const composer=page.locator('.agent-workspace textarea');
   if(handoff)await composer.fill((await composer.inputValue())+'\n请在回复中提供一个 fenced sql 代码块，仅包含 SELECT 42 AS total; 作为不执行的语法示例。');
   const draft=await composer.inputValue();assert(!draft.includes('synthetic-secret'));
   await page.waitForFunction(()=>document.querySelector('.agent-workspace footer button')?.disabled===false);
   const sent=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/turns'),{timeout:180000});await composer.press('Enter');const response=await sent;assert(response.ok());
   const path=new URL(response.url()).pathname.replace(/\/turns$/,'');let completed;
   for(let i=0;i<120;i++){completed=await call('GET',path);if([3,4,5].includes(completed.turns.at(-1)?.status))break;await new Promise(r=>setTimeout(r,500));}
   assert.equal(completed.turns.at(-1)?.status,3);assert.equal(turns.length,1);assert.equal(turns[0].prompt,draft);
   assert.equal(completed.approvals.length,0);assert.equal(executions,review?0:1);
   const messages=completed.messages.map(m=>m.value);assert(!messages.some(m=>m.role==='tool'&&/data\.query|execute_query/i.test(m.tool_name||m.name||'')));
   const answer=messages.filter(m=>m.role==='assistant').map(m=>m.content||'').join('\n');
   assert(!answer.includes('synthetic-secret'));
   if(review)assert(/WHERE|全表|所有行|全部.*数据|all rows/i.test(answer),'Model must flag unbounded deletion');else assert(answer.includes('42'));
   await page.waitForFunction(()=>document.querySelector('.agent-composer-toolbar')?.textContent.includes('Shift + Enter'));
   await page.screenshot({path:review?'/tmp/draft-review-live-agent.png':'/tmp/result-share-live-agent.png'});
   if(handoff){
    const block=page.locator('.agent-message.is-assistant .agent-code').filter({hasText:'SELECT 42 AS total;'}).first();
    await block.getByRole('button',{name:'预览填入',exact:true}).click();await dialog.waitFor();
    const proposal=await dialog.locator('textarea').last().inputValue();assert(proposal.includes('SELECT 42 AS total;'));
    await page.screenshot({path:'/tmp/editor-handoff-live-preview.png'});
    await button('替换草稿，不执行').click();await dialog.waitFor({state:'hidden'});
    assert.equal(await page.locator('.webdata-code-editor textarea').inputValue(),proposal);assert.equal(executions,0);
    await page.screenshot({path:'/tmp/editor-handoff-live-applied.png'});console.log('PASS real-model code block → reviewed editor replacement; no SQL execution');
   }
   assert.deepEqual(errors,[]);console.log(review?'PASS deployed draft review → real-model unbounded deletion warning; no execution or approval':'PASS deployed MySQL synthetic result → redacted preview → explicit draft → real-model analysis; no additional query');
  }finally{await Promise.all(captures);await page.screenshot({path:'/tmp/result-share-live-last.png'});await context.close();}
 }finally{
  await browser?.close();
  for(const id of agents){try{const d=await call('GET','/api/v1/agent/sessions/'+id);await call('DELETE','/api/v1/agent/sessions/'+id,{version:d.session.version});}catch{console.error('Agent cleanup failed');process.exitCode=1;}}
  for(const id of sessions){try{await call('DELETE','/api/v1/webdata/sessions/'+id);}catch{console.error('Connection cleanup failed');process.exitCode=1;}}
  if(restore)await call('PUT',`/api/v1/proxies/${restore.id}`,{status:restore.status});
  await api.dispose();
 }
})().catch(e=>{console.error(e.message);process.exitCode=1;});
