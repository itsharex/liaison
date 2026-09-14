const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright'),assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch();try{
 for(const family of ['webdata','webssh','webdesktop']){
  const ctx=await browser.newContext({viewport:{width:1440,height:1000}});const sessions=[];
  await ctx.addInitScript(()=>{localStorage.setItem('liaison-locale','zh-CN');localStorage.setItem('liaison-theme-preference','dark')});
  await ctx.route('**/api/v1/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname;
   if(req.method()==='POST'&&path.endsWith('/session')){sessions.push(req.postDataJSON());await route.fulfill({status:400,json:{code:400,message:'Connection failed'}});return;}
   let data={};if(path===`/api/v1/${family}/proxies/1`)data={proxy_id:1,proxy_name:'Demo',protocol:family==='webdata'?'mysql':family==='webdesktop'?'rdp':'ssh',effective_status:'active',target_host:'server.example',target_port:22,credentials:[{id:7,username:'demo',database:'app',saved:false}]};
   await route.fulfill({json:{code:200,data}});
  });
  const page=await ctx.newPage();await page.goto(`${process.env.E2E_UI_URL}/e2e/access-auth.html?entry=/${family}/1/connections/7`);
  await page.getByRole('dialog').waitFor();assert.equal(sessions.length,0);
  await page.screenshot({path:`/tmp/access-auth-${family}.png`});
  await page.getByRole('dialog').locator('input[type=password]').fill('temporary-fixture');
  await page.getByRole('dialog').getByRole('button',{name:'连接',exact:true}).click();
  await page.waitForFunction(()=>/失败|Connection failed/.test(document.body.textContent));
  assert.equal(sessions.length,1);assert.equal(sessions[0].password,'temporary-fixture');assert(!sessions[0].save_credential);
  assert(!(await page.evaluate(()=>JSON.stringify({...localStorage,...sessionStorage}))).includes('temporary-fixture'));
  console.log('PASS temporary password prompt, no auto-connect or stored secret:',family);await ctx.close();
 }
}finally{await browser.close();}})().catch(e=>{console.error(e);process.exitCode=1});
