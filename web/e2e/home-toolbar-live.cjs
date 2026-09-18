const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs'),assert=require('node:assert/strict');
(async()=>{
 assert(process.env.E2E_BASE_URL&&process.env.E2E_TOKEN_FILE);
 const {token}=JSON.parse(fs.readFileSync(process.env.E2E_TOKEN_FILE,'utf8'));
 const browser=await chromium.launch();
 try{for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const context=await browser.newContext({ignoreHTTPSErrors:true,viewport:{width:1440,height:1000}});
  await context.addInitScript(({token,locale,theme})=>{localStorage.setItem('token',token);localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{token,locale,theme});
  const page=await context.newPage();await page.goto(process.env.E2E_BASE_URL);
  const history=page.getByRole('button',{name:locale==='zh-CN'?'历史会话':'History',exact:true}),user=page.getByRole('button',{name:locale==='zh-CN'?'打开用户菜单':'Open user menu',exact:true});
  await history.waitFor();assert.equal(await user.count(),1);
  for(const width of [1440,390]){
   await page.setViewportSize({width,height:width===390?844:1000});
   const h=await history.boundingBox(),u=await user.boundingBox();assert(h.x+h.width<=u.x&&u.x+u.width<=width);
   await history.click();assert.equal(await history.getAttribute('aria-expanded'),'true');await history.click();
   await user.click();await page.locator('.liaison-header-user-menu').waitFor();await page.keyboard.press('Escape');
   await page.screenshot({path:`/tmp/home-toolbar-live-${locale}-${theme}-${width}.png`});
  }
  await context.close();console.log('PASS deployed history/avatar positioning and menus',locale,theme);
 }}finally{await browser.close();}
})().catch(e=>{console.error(e.message);process.exitCode=1;});
