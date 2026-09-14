// Local component QA, not a real SSH or authentication acceptance test.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch();
 try {
  for(const locale of ['zh-CN','en-US']) for(const theme of ['dark','light']){
   const context=await browser.newContext({viewport:{width:1440,height:1000}});
   context.setDefaultTimeout(15000);
   const zh=locale==='zh-CN';
   await context.addInitScript(({locale,theme})=>{localStorage.setItem('liaison-locale',locale);localStorage.setItem('liaison-theme-preference',theme)},{locale,theme});
   let uploads=0;let denied=true;const errors=[];const listed=[];
   await context.route('**/api/v1/webssh/**',async route=>{
    const url=new URL(route.request().url());let data;
    if(url.pathname.endsWith('/sessions'))data={id:'fixture',home:'/home/demo',can_upload:true};
    else if(url.pathname.endsWith('/list')){listed.push(url.searchParams.get('path'));if(url.searchParams.get('path')==='/home/demo/documents/documents'&&denied)return route.fulfill({status:403,json:{message:'SFTP_FORBIDDEN'}});data=[{name:'documents',directory:true,mode:'drwx------',size:0},{name:'readme.txt',directory:false,mode:'-rw-------',size:24}];}
    else if(url.pathname.endsWith('/preview'))data={text:'Example UTF-8 file.\n示例内容'};
    else if(url.pathname.endsWith('/upload')){uploads++;return route.fulfill({status:409,json:{reason:'SFTP_EXISTS'}});}
    else if(url.pathname.endsWith('/download'))return route.fulfill({contentType:'application/octet-stream',headers:{'Content-Disposition':'attachment; filename=readme.txt'},body:'Example file'});
    await route.fulfill({json:{code:200,data}});
   });
   const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));
   await page.goto(`${process.env.E2E_UI_URL}/e2e/webssh-files.html`);
   console.log('Loaded component',locale,theme);
   const tree=page.getByRole('navigation',{name:zh?'目录树':'Directory tree'});
   await tree.getByRole('button',{name:'demo',exact:true}).waitFor();
   assert.deepEqual(listed,['/home/demo'],'must not recursively scan');
   await tree.getByRole('button',{name:`${zh?'展开':'Expand'} /home/demo/documents`,exact:true}).click();
   await page.waitForFunction(()=>!document.querySelector('.webssh-directory-label')?.disabled);
   assert.deepEqual(listed,['/home/demo','/home/demo/documents']);
   await page.getByRole('button',{name:zh?'编辑路径':'Edit path',exact:true}).click();
   assert.equal(await page.getByRole('textbox',{name:zh?'远程路径':'Remote path'}).inputValue(),'/home/demo','expand must not navigate');
   await page.getByRole('button',{name:'readme.txt',exact:true}).click();
   assert.equal(await page.getByRole('complementary').count(),0,'single click selects only');
   await page.getByRole('button',{name:zh?'预览':'Preview',exact:true}).click();
   await page.getByText('Example UTF-8 file.',{exact:false}).waitFor();
   await page.screenshot({path:`/tmp/liaison-files-workspace-${locale}-${theme}-preview.png`});
   await page.getByRole('button',{name:zh?'关闭预览':'Close preview',exact:true}).click();
   const path=page.getByRole('textbox',{name:zh?'远程路径':'Remote path'});
   await path.fill('/home/demo/documents');await path.press('Enter');
   await page.getByRole('button',{name:zh?'刷新':'Refresh',exact:true}).waitFor({state:'visible'});
   await page.waitForFunction(()=>document.querySelector('.webssh-files-list')?.getAttribute('aria-busy')==='false');
   assert.equal(await tree.locator('[aria-current=location]').getAttribute('title'),'/home/demo/documents');
   await tree.getByRole('button',{name:`${zh?'展开':'Expand'} /home/demo/documents/documents`,exact:true}).click();
   await tree.getByRole('alert').waitFor();assert.equal(await tree.getByRole('status').count(),0);
   denied=false;await tree.getByRole('button',{name:zh?'重试':'Retry',exact:true}).click();await tree.getByRole('alert').waitFor({state:'hidden'});
   const separator=page.getByRole('separator');await separator.focus();await separator.press('ArrowRight');assert.equal(await separator.getAttribute('aria-valuenow'),'236');
   await page.locator('input[type=file]').setInputFiles({name:'readme.txt',mimeType:'text/plain',buffer:Buffer.from('test')});
   await page.getByText(zh?'同名文件已存在，不会覆盖。请修改文件名后再上传。':'A file with this name exists and will not be overwritten.',{exact:true}).waitFor();assert.equal(uploads,1);
   await page.screenshot({path:`/tmp/liaison-sftp-${locale}-${theme}.png`});
   await page.setViewportSize({width:390,height:844});
   await page.getByRole('button',{name:zh?'目录':'Directories',exact:true}).click();
   await page.screenshot({path:`/tmp/liaison-sftp-${locale}-${theme}-mobile.png`});
   await page.getByRole('textbox',{name:zh?'筛选当前目录':'Filter this folder'}).fill('missing');
   await page.getByText(zh?'没有匹配的文件':'No matching files',{exact:true}).waitFor();
   assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));assert.deepEqual(errors,[]);
   console.log('PASS component',locale,theme,'preview/path/upload-error/mobile');await context.close();
  }
 }finally{await browser.close()}
})().catch(e=>{console.error(e.message);process.exitCode=1});
