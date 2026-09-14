const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try {for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{locale,theme});
  const commands=[],errors=[];let fail=false;
  await context.route('**/api/v1/**',async route=>{
   const path=new URL(route.request().url()).pathname;let data={};
   if(path.endsWith('/webdata/proxies/101'))data={protocol:'s3',proxy_name:'Object storage',target_host:'storage.example',target_port:9000,credentials:[{id:7,username:'fixture-access',saved:true,protocol:'s3'}]};
   else if(path.endsWith('/session'))data={token:'storage-fixture',protocol:'s3'};
   else if(path.endsWith('/execute')){
    const command=JSON.parse(route.request().postDataJSON().statement);commands.push(command);
    if(fail)return route.fulfill({json:{code:503,message:'Unavailable'}});
    data=command.operation==='list_buckets'?{rows:[{kind:'bucket',key:'demo'}]}:command.prefix?{rows:[]}:command.continuation_token?{rows:[{kind:'object',key:'second.txt',size:20}]}:{rows:[{kind:'prefix',key:'photos//中文/'},{kind:'object',key:'report.txt',size:1024,etag:'fixture-etag',modified_at:'2026-09-14T00:00:00Z'},...Array.from({length:60},(_,i)=>({kind:'object',key:`archive-${i}.txt`,size:20}))],next_token:'opaque+/='};
   }
   await route.fulfill({json:{code:200,data}});
  });
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`${process.env.E2E_UI_URL}/e2e/webs3.html`);
  const zh=locale==='zh-CN',button=(cn,en)=>page.getByRole('button',{name:zh?cn:en,exact:true});
  await button('demo','demo').click();await button('report.txt','report.txt').waitFor();
  assert.equal(await page.locator('input[type=password]').count(),0);
  await button('下一页','Next').click();await button('second.txt','second.txt').waitFor();
  assert.equal(commands.at(-1).continuation_token,'opaque+/=');
  await button('上一页','Previous').click();await button('report.txt','report.txt').click();await page.getByText('fixture-etag',{exact:true}).waitFor();
  assert(await button('下一页','Next').isEnabled(),'returning to previous page must permit forward navigation');
  await page.screenshot({path:`/tmp/webs3-${locale}-${theme}.png`});
  const sideBefore=await page.locator('.webs3-sidebar').boundingBox();
  await page.locator('.webs3-table').evaluate(el=>{el.scrollTop=el.scrollHeight;});
  assert(await page.locator('.webs3-table').evaluate(el=>el.scrollTop>0),'object list must scroll independently');
  assert.deepEqual(await page.locator('.webs3-sidebar').boundingBox(),sideBefore,'sidebar must stay fixed');
  assert(await button('下一页','Next').isVisible(),'pagination stays reachable');
  await page.screenshot({path:`/tmp/webs3-scrolled-${locale}-${theme}.png`});
  await button('photos//中文/','photos//中文/').last().click();await page.getByText(zh?'此处没有对象。':'No objects here.',{exact:true}).waitFor();
  assert.equal(commands.at(-1).prefix,'photos//中文/');
  await button('上一级','Up').click();await button('report.txt','report.txt').waitFor();
  fail=true;await button('刷新','Refresh').click();await page.getByText(zh?'对象列举失败，请检查权限后重试。':'Could not list objects. Check permissions and retry.',{exact:true}).waitFor();
  fail=false;await button('刷新','Refresh').click();await button('report.txt','report.txt').waitFor();
  await page.setViewportSize({width:390,height:844});
  await page.waitForFunction(()=>document.querySelector('.webs3-sidebar').getBoundingClientRect().height<=161);
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page overflow');
  await page.screenshot({path:`/tmp/webs3-mobile-${locale}-${theme}.png`});
  assert.deepEqual(errors,[]);console.log('PASS WebS3 listing, pagination, prefixes, retry, layout',locale,theme);await context.close();
 }}finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
