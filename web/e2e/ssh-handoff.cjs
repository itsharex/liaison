const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try{for(const locale of ['zh-CN','en-US'])for(const theme of ['dark','light']){
  const context=await browser.newContext({viewport:{width:1440,height:1000}});
  await context.addInitScript(({locale,theme})=>{
   localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme);
   window.sshSent=[];
   window.WebSocket=class {
    static OPEN=1;readyState=1;
    constructor(){window.sshSocket=this;setTimeout(()=>{this.onopen?.();this.emit({type:'status',status:'connected'});this.emit({type:'output',data:'\r\n\x1b]633;A\x07demo$ \x1b]633;B\x07'});},100);}
    emit(msg){this.onmessage?.({data:JSON.stringify(msg)});}
    send(raw){const msg=JSON.parse(raw);if(msg.type==='input'){window.sshSent.push(msg.data);this.emit({type:'output',data:msg.data});}}
    close(){this.readyState=3;this.onclose?.();}
   };
  },{locale,theme});
  const detail={session:{id:'session_abcdef',kind:'access',status:0},attachments:[{id:'ssh-fixture',access_id:101}],messages:[{id:'answer',value:{role:'assistant',content:'```sh\necho hello\n```'}}],turns:[],approvals:[],steps:[]};
  await context.route('**/api/v1/**',async route=>{
   const path=new URL(route.request().url()).pathname;let data={};
   if(path.endsWith('/agent/status'))data={enabled:true};
   else if(path.endsWith('/events'))return route.fulfill({contentType:'text/event-stream',body:': keepalive\n\n'});
   else if(path.includes('/agent/sessions'))data=detail;
   else if(path.endsWith('/webssh/proxies/101'))data={proxy_name:'SSH workspace',target_host:'shell.example',target_port:22,effective_status:'active',credentials:[{id:7,username:'demo',saved:true}]};
   else if(path.endsWith('/session'))data={token:'ssh-fixture',ws_url:'ws://fixture.invalid'};
   await route.fulfill({json:{code:200,data}});
  });
  const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
  const zh=locale==='zh-CN',button=(cn,en)=>page.getByRole('button',{name:zh?cn:en,exact:true});
  await page.goto(`${process.env.E2E_UI_URL}/e2e/ssh-handoff.html`);
  await button('Agent','Agent').waitFor({timeout:15000});
  assert(await button('分享选中输出','Share selected output').isDisabled());
  const screen=await page.locator('.xterm-screen').boundingBox();
  await page.mouse.click(screen.x+35,screen.y+7,{clickCount:3});
  await button('分享选中输出','Share selected output').click();
  const shareDialog=page.getByRole('dialog');const sample=shareDialog.locator('textarea');assert((await sample.inputValue()).trim().length>0);
  await sample.fill('汉'.repeat(5000));assert(await button('放入 Agent 草稿','Add to Agent draft').isDisabled());
  await sample.fill('Selected diagnostic sample');
  await page.screenshot({path:`/tmp/ssh-output-share-${locale}-${theme}.png`});
  await button('放入 Agent 草稿','Add to Agent draft').click();await shareDialog.waitFor({state:'hidden'});
  await page.waitForFunction(()=>document.querySelector('.agent-workspace textarea')?.value.includes('Selected diagnostic sample'));
  assert.deepEqual(await page.evaluate(()=>window.sshSent),[],'Sharing does not send terminal input');
  await page.locator('.agent-workspace > header button').last().click();
  await page.mouse.click(screen.x+35,screen.y+7);
  await button('Agent','Agent').click();
  await button('预览填入','Preview in editor').click();
  const dialog=page.getByRole('dialog'),text=dialog.locator('textarea');await dialog.waitFor();
  assert.equal(await text.inputValue(),'echo hello');
  await text.fill('echo one\necho two');assert(await button('填入，不执行','Insert, do not run').isDisabled());
  await text.fill('echo hello');assert(await button('填入，不执行','Insert, do not run').isEnabled());
  await page.screenshot({path:`/tmp/ssh-handoff-${locale}-${theme}-desktop.png`});
  await page.setViewportSize({width:390,height:844});await page.screenshot({path:`/tmp/ssh-handoff-${locale}-${theme}-mobile.png`});
  assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1));
  await button('填入，不执行','Insert, do not run').click();await dialog.waitFor({state:'hidden'});
  await page.waitForFunction(()=>window.sshSent.length>0);assert.equal(await page.evaluate(()=>window.sshSent.join('')),'echo hello');
  await page.locator('.agent-workspace').waitFor({state:'hidden'});assert.deepEqual(errors,[]);
  await context.close();console.log('PASS SSH preview, multiline rejection, exact insertion without Enter:',locale,theme);
 }}finally{await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
