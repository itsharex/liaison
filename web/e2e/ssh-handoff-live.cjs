// Real staging SSH + model. Only inserts a reviewed printf draft; never executes it.
const {chromium,request}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 assert(process.env.E2E_BASE_URL&&process.env.E2E_TOKEN_FILE);
 const {token}=JSON.parse(fs.readFileSync(process.env.E2E_TOKEN_FILE,'utf8'));
 const api=await request.newContext({baseURL:process.env.E2E_BASE_URL,ignoreHTTPSErrors:true,extraHTTPHeaders:{Authorization:`Bearer ${token}`}});
 const call=async(method,path,data)=>{const r=await api.fetch(path,{method,data});assert(r.ok(),`${method} request failed: ${r.status()}`);const b=await r.json();assert.equal(b.code,200);return b.data;};
 const browser=await chromium.launch();const agents=new Set(),handles=new Set();let page;
 try{
  const proxies=(await call('GET','/api/v1/proxies?page_size=200')).proxies;
  let selected,credential;
  for(const proxy of proxies.filter(p=>p.access_protocol==='webssh'&&p.status==='running')){
   const target=await call('GET',`/api/v1/webssh/proxies/${proxy.id}`);
   credential=target.credentials?.find(c=>c.saved);if(credential){selected=proxy;break;}
  }
  assert(selected&&credential,'An enabled SSH demo with saved credentials is required');
  const context=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000}});
  await context.addInitScript(token=>{localStorage.setItem('token',token);localStorage.setItem('liaison-locale','zh-CN');localStorage.setItem('liaison-theme-preference','dark');},token);
  page=await context.newPage();page.setDefaultTimeout(30000);
  let output='',sent='';const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  page.on('websocket',ws=>{ws.on('framereceived',e=>{try{const v=JSON.parse(String(e.payload));if(v.type==='output')output+=v.data;}catch{}});ws.on('framesent',e=>{try{const v=JSON.parse(String(e.payload));if(v.type==='input')sent+=v.data;}catch{}});});
  page.on('response',r=>{if(r.request().method()!=='POST'||!r.ok())return;if(r.url().endsWith('/agent/sessions'))void r.json().then(b=>{if(b.data)agents.add(b.data.session.id);});else if(r.url().endsWith('/session'))void r.json().then(b=>{if(b.data?.token)handles.add(b.data.token);});});
  const entry=`/webssh/${selected.id}/connections/${credential.id}`;
  await page.goto(process.env.E2E_BASE_URL+entry);
  const until=async(fn)=>{const end=Date.now()+30000;while(!fn()){assert(Date.now()<end,'Timed out waiting for SSH state');await page.waitForTimeout(100);}};
  await until(()=>output.includes('633;B'));
  await page.getByRole('button',{name:'关闭自动提示',exact:true}).click();
  await page.getByRole('button',{name:'Agent',exact:true}).click();
  const composer=page.locator('.agent-workspace textarea');await composer.waitFor();
  const marker='handoff-'+Date.now();const expected=`printf '${marker}'`;
  await composer.fill(`只回复一个 sh 代码块，代码精确为 ${expected}。不要调用工具，不要执行命令，不要添加其他内容。`);
  await page.waitForFunction(()=>!document.querySelector('.agent-workspace footer button')?.disabled);
  await composer.press('Enter');
  const preview=page.getByRole('button',{name:'预览填入',exact:true});await preview.waitFor({timeout:120000});await preview.click();
  const dialog=page.getByRole('dialog');assert.equal((await dialog.locator('textarea').inputValue()).trim(),expected,'Never insert unexpected model output');
  const before=sent.length;await page.screenshot({path:'/tmp/ssh-handoff-real-preview.png'});
  await page.getByRole('button',{name:'填入，不执行',exact:true}).click();await dialog.waitFor({state:'hidden'});await until(()=>sent.length>before);
  assert.equal(sent.slice(before),expected,'Only command text, no Enter');
  await page.locator('.webssh-terminal-screen').click();await page.keyboard.press('Control+c');
  const screen=await page.locator('.xterm-screen').boundingBox();await page.mouse.click(screen.x+35,screen.y+7,{clickCount:3});
  await page.getByRole('button',{name:'分享选中输出',exact:true}).click();
  const share=page.getByRole('dialog');assert((await share.locator('textarea').inputValue()).trim());
  await share.locator('textarea').fill('Synthetic selected output for acceptance');
  await page.getByRole('button',{name:'放入 Agent 草稿',exact:true}).click();
  await page.waitForFunction(()=>document.querySelector('.agent-workspace textarea')?.value.includes('Synthetic selected output'));
  const sessionURL=page.url();const count=handles.size;await page.reload();await page.locator('.agent-workspace').waitFor();
  assert(page.url().includes('/sessions/'));assert.equal(handles.size,count,'Reload must not silently create another SSH connection');
  assert.equal(await page.locator('.agent-workspace textarea').inputValue(),'','Unsent selection is not durable cross-reload memory');
  assert.equal(await page.getByRole('button',{name:'预览填入',exact:true}).count(),0,'Offline history cannot inject terminal input');
  await page.screenshot({path:'/tmp/ssh-handoff-real-history.png'});
  assert.deepEqual(errors,[]);console.log('PASS real SSH/model reviewed insertion, selected-output draft, refresh/history without reconnect:',new URL(sessionURL).pathname.includes('/sessions/'));
 }finally{
  await browser.close();
  for(const id of agents){const d=await call('GET',`/api/v1/agent/sessions/${id}`);await call('DELETE',`/api/v1/agent/sessions/${id}`,{version:d.session.version});}
  await api.dispose();
 }
})().catch(e=>{console.error(e.message);process.exitCode=1;});
