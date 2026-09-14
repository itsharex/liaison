const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright'),assert=require('node:assert/strict');
(async()=>{const browser=await chromium.launch();try{for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
const ctx=await browser.newContext({viewport:{width:1440,height:1000}}),saved=[];
await ctx.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme)},{locale,theme});
await ctx.route('**/api/v1/**',async r=>{let data={protocol:'openai-compatible',base_path:'/v1',tls:false,has_api_key:false};if(r.request().method()==='PUT'){data=r.request().postDataJSON();saved.push(data)}await r.fulfill({json:{code:200,data}})});
const p=await ctx.newPage();await p.goto(`${process.env.E2E_UI_URL}/e2e/ollama.html`);const select=p.locator('select').first();await select.selectOption('ollama');await p.getByPlaceholder('/api').waitFor();assert.equal(await p.getByPlaceholder('/api').inputValue(),'/api');
await p.getByRole('button',{name:locale==='zh-CN'?'保存':'Save',exact:true}).click();assert.equal(saved[0].protocol,'ollama');assert.equal(saved[0].base_path,'/api');await p.screenshot({path:`/tmp/ollama-${locale}-${theme}.png`});
await p.getByPlaceholder('/api').fill('/custom/api');await select.selectOption('anthropic');assert.equal(await p.getByPlaceholder('/v1').inputValue(),'/custom/api');
await p.setViewportSize({width:390,height:844});assert(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await p.screenshot({path:`/tmp/ollama-${locale}-${theme}-mobile.png`});console.log('PASS Ollama configuration',locale,theme);await ctx.close();
}}finally{await browser.close()}})().catch(e=>{console.error(e);process.exitCode=1});
