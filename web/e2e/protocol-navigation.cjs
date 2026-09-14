const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright'),assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch();try{for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
 const zh=locale==='zh-CN',ctx=await browser.newContext({viewport:{width:1280,height:900}}),errors=[];
 await ctx.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);},{locale,theme});
 await ctx.route('**/api/v1/**',r=>r.fulfill({json:{code:200,data:new URL(r.request().url()).pathname.endsWith('/audits/access')?{items:[],total:0}:{applications:[],proxies:[]}}}));
 const page=await ctx.newPage();page.on('pageerror',e=>errors.push(e.message));const base=(process.env.E2E_UI_URL||'http://127.0.0.1:8000')+'/e2e/protocol-navigation.html';
 const more=()=>page.getByRole('button',{name:zh?'更多协议':'More protocols',exact:true});
 const choose=async name=>{const tab=page.locator('.liaison-overflow-tabs nav').getByRole('button',{name,exact:true});if(await tab.count())await tab.click();else{await more().click();await page.getByRole('menuitem',{name,exact:true}).click();}await tab.waitFor();assert.equal(await tab.getAttribute('aria-pressed'),'true');};
 for(const screen of ['audit','access']){
 await page.setViewportSize({width:1280,height:900});await page.goto(base+(screen==='audit'?'?screen=audit&protocol=webmysql&keyword=report&success=false':'?category=database&name=demo'));
 await page.locator('.liaison-overflow-tabs nav button').first().waitFor();await choose('WebMongoDB');assert.equal(new URL(page.url()).searchParams.get(screen==='audit'?'protocol':'access_type'),screen==='audit'?'mongodb':'webmongodb');
 await choose('WebMySQL');await page.goBack();await page.locator('.liaison-overflow-tabs nav button[aria-pressed=true]').filter({hasText:'WebMongoDB'}).waitFor();await page.reload();await page.locator('.liaison-overflow-tabs nav button[aria-pressed=true]').filter({hasText:'WebMongoDB'}).waitFor();
 await page.screenshot({path:`/tmp/overflow-${screen}-${locale}-${theme}.png`,fullPage:true});
 await page.setViewportSize({width:390,height:844});await more().waitFor();await choose('WebOpenSearch');assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));await more().click();assert(await page.getByRole('menu').isVisible());await page.keyboard.press('ArrowDown');assert(await page.getByRole('menu').evaluate(el=>el.contains(document.activeElement)));await page.screenshot({path:`/tmp/overflow-${screen}-mobile-${locale}-${theme}.png`,fullPage:true});await page.keyboard.press('Escape');assert.equal(await page.getByRole('menu').count(),0);assert(await more().evaluate(el=>el===document.activeElement));
 await page.setViewportSize({width:2400,height:900});await page.waitForTimeout(100);assert.equal(await more().count(),0,'all tabs fit on wide screen');assert.equal(new URL(page.url()).searchParams.get(screen==='audit'?'keyword':'name'),screen==='audit'?'report':'demo');
 }
 assert.deepEqual(errors,[]);console.log('PASS overflow, selected tab, resize, keyboard, URL/back/refresh',locale,theme);await ctx.close();
}}finally{await browser.close();}})().catch(e=>{console.error(e);process.exitCode=1;});
