const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try {for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{locale,theme});
  const commands=[],errors=[],order=[];let fail=false,uploadConflict=false,contextFail=false,expired=false;
  const detail={session:{id:'session_abcdef',kind:'access',status:0},attachments:[{id:'storage-fixture',access_id:101}],messages:[],turns:[],approvals:[],steps:[]};
  await context.route('**/api/v1/**',async route=>{
   const path=new URL(route.request().url()).pathname;let data={};
   if(path.endsWith('/agent/status'))data={enabled:true};
   else if(path.endsWith('/events'))return route.fulfill({contentType:'text/event-stream',body:': keepalive\n\n'});
   else if(path.endsWith('/turns')){order.push('turn');data={};}
   else if(path.includes('/agent/sessions'))data=detail;
   else if(path.endsWith('/capabilities'))data={can_upload:true,transfer_limit:16*1024*1024};
   else if(path.endsWith('/context')){if(contextFail)return route.fulfill({status:503,json:{code:503}});order.push(route.request().postDataJSON());}
   else if(path.endsWith('/download'))return route.fulfill({contentType:'application/octet-stream',body:'fixture payload'});
   else if(path.endsWith('/upload')){if(uploadConflict)return route.fulfill({status:409,json:{code:409}});order.push('upload');}
   else if(path.endsWith('/webdata/proxies/101'))data={protocol:'s3',proxy_name:'Object storage',target_host:'storage.example',target_port:9000,credentials:[{id:7,username:'fixture-access',saved:true,protocol:'s3'}]};
   else if(path.endsWith('/session'))data={token:'storage-fixture',protocol:'s3'};
   else if(path.endsWith('/execute')){
    if(expired)return route.fulfill({status:401,json:{code:401}});
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
  const downloaded=page.waitForEvent('download');await button('下载','Download').click();assert.equal((await downloaded).suggestedFilename(),'report.txt');
  await page.locator('input[type=file]').setInputFiles({name:'notes.txt',mimeType:'text/plain',buffer:Buffer.from('fixture')});
  await page.getByRole('dialog').waitFor();
  uploadConflict=true;await button('确认上传','Confirm upload').click();await page.getByRole('dialog').getByText(zh?'对象已存在，未覆盖。请更换名称。':'Object already exists; nothing was overwritten. Choose another name.',{exact:true}).waitFor();
  await page.screenshot({path:`/tmp/webs3-upload-${locale}-${theme}.png`});
  uploadConflict=false;await button('确认上传','Confirm upload').click();await page.getByRole('dialog').waitFor({state:'hidden'});assert(order.includes('upload'));
  await button('report.txt','report.txt').click();await button('询问 Agent','Ask Agent').click();
  const composer=page.locator('.agent-workspace textarea');await composer.waitFor();
  await composer.fill('Explain the selected object');
  await page.waitForFunction(()=>document.querySelector('.agent-workspace footer button')?.disabled===false);
  contextFail=true;await composer.press('Enter');await page.getByText(zh?'无法同步当前上下文，请检查连接后重试。':'Could not sync current context. Check the connection and retry.',{exact:true}).waitFor();assert(!order.includes('turn'),'context failure must stop inference');
  contextFail=false;await composer.press('Enter');await page.waitForTimeout(200);assert.equal(order.at(-1),'turn');assert.deepEqual(order.at(-2),{bucket:'demo',prefix:'',key:'report.txt'});
  await page.screenshot({path:`/tmp/webs3-agent-${locale}-${theme}.png`});
  const layout=await page.locator('.agent-workspace').evaluate(el=>['header','.agent-workspace-body','footer'].map(selector=>{const r=el.querySelector(selector).getBoundingClientRect();return {top:r.top,bottom:r.bottom};}));
  assert(layout[0].bottom<=layout[1].top+1&&layout[1].bottom<=layout[2].top+1,'Agent regions must not overlap');
  await page.locator('.agent-workspace > header button').last().click();
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
  await button('Agent','Agent').click();await page.locator('.agent-workspace').waitFor();
  await page.screenshot({path:`/tmp/webs3-agent-mobile-${locale}-${theme}.png`});
  await page.locator('.agent-workspace > header button').last().click();
  expired=true;await button('刷新','Refresh').click();await page.getByText(zh?'连接已失效，请重新连接后继续。':'Connection unavailable. Reconnect to continue.',{exact:true}).waitFor();
  expired=false;await button('重新连接','Reconnect').click();await button('demo','demo').waitFor();
  assert.deepEqual(errors,[]);console.log('PASS WebS3 listing, pagination, prefixes, retry, layout',locale,theme);await context.close();
 }}finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
