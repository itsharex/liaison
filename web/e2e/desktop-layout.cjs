const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright'),assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch();try{for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
const context=await browser.newContext({viewport:{width:1440,height:1000}});
await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme)},{locale,theme});
await context.route('**/api/v1/**',r=>r.fulfill({json:{code:200,data:{}}}));
const p=await context.newPage();await p.goto(`${process.env.E2E_UI_URL}/e2e/desktop-layout.html`);await p.locator('.agent-workspace-root.is-docked').waitFor();
const desktop=await p.locator('.webdesktop-shell').boundingBox(),agent=await p.locator('.agent-workspace').boundingBox(),header=await p.locator('.webdesktop-toolbar').boundingBox();
assert(agent.x>=desktop.x+desktop.width+15);assert(Math.abs(agent.y-desktop.y)<1);assert(Math.abs(agent.height-desktop.height)<1);assert(header.y+header.height<desktop.y);
await p.screenshot({path:`/tmp/desktop-layout-${locale}-${theme}.png`});
await p.getByRole('button',{name:'Agent',exact:true}).click();assert.equal(await p.locator('.agent-workspace-root').count(),0);assert((await p.locator('.webdesktop-shell').boundingBox()).width>desktop.width);
await p.setViewportSize({width:390,height:844});await p.getByRole('button',{name:'Agent',exact:true}).click();await p.locator('.agent-workspace-mask').waitFor();assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await p.screenshot({path:`/tmp/desktop-layout-${locale}-${theme}-mobile.png`});
console.log('PASS desktop frame, dock/close/mobile,',locale,theme);await context.close();
}}finally{await browser.close()}})().catch(e=>{console.error(e);process.exitCode=1});
