// Browser fixtures verify navigation handoff, not real upstream/model inference.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try {
  for(const protocol of ['mysql','mongodb','elasticsearch','redis','memcached']) for(const locale of ['zh-CN','en-US']) for(const theme of ['dark','light']) {
   const context=await browser.newContext({viewport:{width:1440,height:1000}});
   await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{locale,theme});
   const order=[],errors=[];let fail=false;
   const detail={session:{id:'session_abcdef',kind:'access',status:0},attachments:[{id:'data-fixture',access_id:101}],messages:[],turns:[],approvals:[],steps:[]};
   const objectType={mysql:'table',mongodb:'collection',elasticsearch:'index',redis:'key'}[protocol];
   const node=name=>({key:name,title:name,type:objectType,is_leaf:true,meta:protocol==='redis'?{key:name}:{database:'demo',name}});
   await context.route('**/api/v1/**',async route=>{
    const path=new URL(route.request().url()).pathname;let data={};
    if(path.endsWith('/agent/status'))data={enabled:true};
    else if(path.endsWith('/events'))return route.fulfill({contentType:'text/event-stream',body:': keepalive\n\n'});
    else if(path.endsWith('/turns'))order.push('turn');
    else if(path.includes('/agent/sessions'))data=detail;
    else if(path.endsWith('/context')) {if(fail)return route.fulfill({status:503,json:{code:503}});order.push(route.request().postDataJSON());}
    else if(path.endsWith('/webdata/proxies/101'))data={protocol,proxy_name:'Data workspace',target_host:'data.example',target_port:3306,effective_status:'active',credentials:[{id:7,protocol,name:'Fixture',username:'demo',saved:true,database:'demo'}]};
    else if(path.endsWith('/session'))data={token:'data-fixture',protocol};
    else if(path.endsWith('/metadata'))data={nodes:protocol==='memcached'?[]:[node('orders'),node('customers')]};
    else if(path.endsWith('/object'))data={type:objectType,name:new URL(route.request().url()).searchParams.get('name')||'orders',database:'demo',columns:[],indexes:[]};
    else if(path.endsWith('/execute'))data={type:'rows',columns:[],rows:[]};
    await route.fulfill({json:{code:200,data}});
   });
   const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
   await page.goto(`${process.env.E2E_UI_URL}/e2e/data-context.html`);
   const zh=locale==='zh-CN';
   if(protocol==='redis') {
    await page.locator('.webdata-object-tree').getByText('DB 0',{exact:true}).waitFor();
    for(let depth=0;depth<2;depth++) {
     const expand=page.locator('.webdata-object-tree').getByRole('button',{name:'Expand',exact:true});
     if(await expand.count())await expand.first().click();
    }
   }
   if(protocol==='memcached')await page.getByRole('textbox',{name:'Key',exact:true}).fill('orders');
   else await page.locator('.webdata-object-tree').getByText('orders',{exact:true}).click();
   await page.getByRole('button',{name:'Agent',exact:true}).click();
   const composer=page.locator('.agent-workspace textarea');await composer.waitFor();
   await composer.fill('Explain current object');
   await page.waitForFunction(()=>document.querySelector('.agent-workspace footer button')?.disabled===false);
   fail=true;await composer.press('Enter');
   await page.getByText(zh?'无法同步当前上下文，请检查连接后重试。':'Could not sync current context. Check the connection and retry.',{exact:true}).waitFor();
   assert(!order.includes('turn'),'failed sync must block inference');
   fail=false;const firstTurn=page.waitForResponse(r=>r.url().endsWith('/turns'));await composer.press('Enter');await firstTurn;
   await page.locator('.agent-composer-toolbar').getByText(zh?'Shift + Enter 换行':'Shift + Enter for a new line',{exact:true}).waitFor();
   assert.equal(order.at(-1),'turn');assert.equal(order.at(-2).name,'orders');
   assert.deepEqual(Object.keys(order.at(-2)).sort(),['database','name','object_type','schema']);
   if(protocol==='memcached')await page.getByRole('textbox',{name:'Key',exact:true}).fill('customers');
   else await page.locator('.webdata-object-tree').getByText('customers',{exact:true}).click();
   await composer.fill('And this one?');const secondTurn=page.waitForResponse(r=>r.url().endsWith('/turns'));await composer.press('Enter');await secondTurn;
   await page.locator('.agent-composer-toolbar').getByText(zh?'Shift + Enter 换行':'Shift + Enter for a new line',{exact:true}).waitFor();
   assert.equal(order.at(-2).name,'customers','each turn must replace previous selection');
   assert((await page.locator('.agent-workspace-context').innerText()).includes('customers'));
   await page.locator('.liaison-toast').waitFor({state:'hidden'});
   await page.screenshot({path:`/tmp/data-context-${protocol}-${locale}-${theme}.png`});
   await page.setViewportSize({width:390,height:844});
   await page.screenshot({path:`/tmp/data-context-mobile-${protocol}-${locale}-${theme}.png`});
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'no page overflow');
   assert.deepEqual(errors,[]);await context.close();console.log('PASS',protocol,locale,theme);
  }
 } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exit(1);});
