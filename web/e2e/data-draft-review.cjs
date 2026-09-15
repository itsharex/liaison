const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try{for(const protocol of ['mysql','mongodb','redis','elasticsearch'])for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{locale,theme});
  const turns=[],errors=[];let executions=0,suggestions=0;
  const detail={session:{id:'session_abcdef',kind:'access',status:0},attachments:[{id:'data-fixture',access_id:101}],messages:[],turns:[],approvals:[],steps:[]};
  await context.route('**/api/v1/**',async route=>{
   const path=new URL(route.request().url()).pathname;let data={};
   if(path.endsWith('/agent/status'))data={enabled:true};
   else if(path.endsWith('/events'))return route.fulfill({contentType:'text/event-stream',body:': keepalive\n\n'});
   else if(path.endsWith('/turns'))turns.push(route.request().postDataJSON());
   else if(path.includes('/agent/sessions'))data=detail;
   else if(path.endsWith('/webdata/proxies/101'))data={protocol,proxy_name:'Data workspace',target_host:'data.example',target_port:3306,effective_status:'active',credentials:[{id:7,protocol,name:'Fixture',username:'demo',saved:true,database:'demo'}]};
   else if(path.endsWith('/session'))data={token:'data-fixture',protocol};
   else if(path.endsWith('/metadata'))data={nodes:[]};
   else if(path.endsWith('/execute'))executions++;
   else if(path.endsWith('/suggestions')&&route.request().method()==='POST')suggestions++;
   await route.fulfill({json:{code:200,data}});
  });
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`${process.env.E2E_UI_URL}/e2e/data-context.html`);
  const zh=locale==='zh-CN',button=(cn,en)=>page.getByRole('button',{name:zh?cn:en,exact:true});
  const editor=page.locator('.webdata-code-editor textarea');await editor.fill('');assert(await button('审阅草稿','Review draft').isDisabled());
  const original={mysql:'DELETE FROM orders;',mongodb:'db.orders.deleteMany({})',redis:'FLUSHALL',elasticsearch:'DELETE /orders'}[protocol];
  await editor.fill(original+' /* synthetic-private-note */');await button('审阅草稿','Review draft').click();
  const dialog=page.getByRole('dialog'),preview=dialog.locator('textarea');await dialog.waitFor();assert.equal(await preview.inputValue(),original+' /* synthetic-private-note */');
  await preview.fill('changed');await button('取消','Cancel').click();assert.equal(await editor.inputValue(),original+' /* synthetic-private-note */');assert.equal(turns.length,0);
  await button('Agent','Agent').click();const composer=page.locator('.agent-workspace textarea');await composer.fill('Existing question');await page.locator('.agent-workspace > header button').last().click();
  await button('审阅草稿','Review draft').click();await preview.fill('汉'.repeat(5000));assert(await button('放入 Agent 草稿','Add to Agent draft').isDisabled());
  await preview.fill(original);await page.waitForFunction(()=>document.querySelectorAll('.liaison-toast').length===0);
  if(protocol==='mysql'){
   await page.screenshot({path:`/tmp/draft-review-${locale}-${theme}.png`});
   await page.setViewportSize({width:390,height:844});await page.screenshot({path:`/tmp/draft-review-mobile-${locale}-${theme}.png`});
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  }
  await button('放入 Agent 草稿','Add to Agent draft').click();await dialog.waitFor({state:'hidden'});
  await page.waitForFunction(()=>document.querySelector('.agent-workspace textarea')?.value.includes('User-reviewed editor snapshot'));
  const draft=await composer.inputValue();assert(draft.startsWith('Existing question'));assert(draft.includes(original)&&draft.includes(protocol)&&!draft.includes('synthetic-private-note'));assert.equal(turns.length,0);assert.equal(executions,0);assert.equal(suggestions,0);
  await page.setViewportSize({width:1440,height:1000});assert.equal(await editor.inputValue(),original+' /* synthetic-private-note */');
  await page.waitForFunction(()=>document.querySelector('.agent-workspace footer button')?.disabled===false);
  const sent=page.waitForResponse(r=>r.url().endsWith('/turns'));await composer.press('Enter');await sent.catch(async error=>{await page.screenshot({path:'/tmp/draft-review-send-failure.png'});console.error({protocol,locale,theme,composer:await composer.inputValue(),workspace:await page.locator('.agent-workspace').innerText(),errors});throw error;});
  assert.equal(turns.length,1);assert.equal(turns[0].prompt,draft);assert.equal(executions,0);assert.equal(suggestions,0);assert.deepEqual(errors,[]);
  await page.goto(`${process.env.E2E_UI_URL}/e2e/data-context.html?ai=0`);await editor.waitFor();assert.equal(await button('审阅草稿','Review draft').count(),0);
  await context.close();console.log('PASS draft review, edit isolation, manual sharing, no execution, permission UI:',protocol,locale,theme);
 }}finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
