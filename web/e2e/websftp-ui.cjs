// File access has no connection manager. Only fixtures; no real SSH credentials.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright'),assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch();try{
 for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const ctx=await browser.newContext({viewport:{width:1440,height:1000}}),zh=locale==='zh-CN';let fail=true,missing=false,saved=true;const sessions=[],errors=[];
  await ctx.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme)},{locale,theme});
  await ctx.route('**/api/v1/webssh/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname;let data;
   assert(!path.endsWith('/session'),'no PTY');assert(!path.endsWith('/credential'),'no connection mutation');
   if(path==='/api/v1/webssh/proxies/1')data={proxy_id:1,access_protocol:'websftp',effective_status:'active',credentials:missing?[]:[{id:7,username:'demo',saved}]};
   else if(req.method()==='POST'){
    sessions.push(req.postDataJSON());if(fail){await route.fulfill({status:502,json:{code:502}});return;}
    data={id:'fixture',home:'/home/demo',can_upload:true};
   }else if(path.endsWith('/list'))data=[{name:'readme.txt',directory:false,mode:'-rw-------',size:12}];
   else if(path.endsWith('/preview'))data={text:'Example file'};
   await route.fulfill({json:{code:200,data}});
  });
  const page=await ctx.newPage();page.on('pageerror',e=>errors.push(e.message));
  await page.goto(`${process.env.E2E_UI_URL}/e2e/websftp.html`);await page.getByRole('button',{name:zh?'重试':'Retry',exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:zh?'新建连接':'New connection',exact:true}).count(),0);
  fail=false;await page.getByRole('button',{name:zh?'重试':'Retry',exact:true}).click();
  await page.getByRole('button',{name:'readme.txt',exact:true}).waitFor();assert.equal(sessions.at(-1).use_saved_credential,true);assert.equal(sessions.at(-1).username,'demo');
  await page.getByRole('button',{name:'readme.txt',exact:true}).dblclick();await page.getByText('Example file',{exact:true}).waitFor();
  await page.screenshot({path:`/tmp/sftp-direct-${locale}-${theme}.png`});
  await page.setViewportSize({width:390,height:844});await page.screenshot({path:`/tmp/sftp-direct-${locale}-${theme}-mobile.png`});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  saved=false;await page.goto(`${process.env.E2E_UI_URL}/e2e/websftp.html`);await page.getByRole('dialog').waitFor();
  const before=sessions.length;assert.equal(await page.locator('input[readonly]').inputValue(),'demo');
  await page.locator('input[type=password]').fill('temporary-fixture');fail=true;
  await page.getByRole('button',{name:zh?'连接':'Connect',exact:true}).click();await page.locator('.liaison-notice').waitFor();
  fail=false;await page.getByRole('button',{name:zh?'连接':'Connect',exact:true}).click();await page.getByRole('button',{name:'readme.txt',exact:true}).waitFor();
  assert.equal(sessions.length,before+2);assert.equal(sessions.at(-1).password,'temporary-fixture');assert.equal(sessions.at(-1).save_credential,false);
  assert(!(await page.evaluate(()=>JSON.stringify({...localStorage,...sessionStorage}))).includes('temporary-fixture'));
  missing=true;await page.goto(`${process.env.E2E_UI_URL}/e2e/websftp.html`);await page.locator('output').waitFor();assert((await page.locator('output').textContent()).includes('/proxy?access_type=websftp&configure=1'));
  assert.deepEqual(errors,[]);console.log('PASS',locale,theme,'direct file access/retry/preview/mobile/missing config redirect');await ctx.close();
 }
}finally{await browser.close();}})().catch(e=>{console.error(e);process.exitCode=1});
