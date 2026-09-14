// Browser interaction fixture. Real connector/service acceptance is separate.
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
  const browser = await chromium.launch();
  try {
    for (const locale of ['zh-CN', 'en-US']) for (const theme of ['dark', 'light']) {
      const context = await browser.newContext({viewport: {width:1440,height:1000}});
      await context.addInitScript(({locale,theme}) => {
        localStorage.setItem('liaison-locale',locale);
        localStorage.setItem('liaison-theme-preference',theme);
      }, {locale,theme});
      const commands=[], errors=[]; let failStats=false, zeroHits=false;
      await context.route('**/api/v1/**', async route => {
        const path = new URL(route.request().url()).pathname;
        let data = {};
        if (path.endsWith('/webdata/proxies/101')) data = {protocol:'memcached',proxy_name:'Cache demo',target_host:'cache.example',target_port:11211,effective_status:'active',credentials:[{id:7,protocol:'memcached',name:'Cache',username:'',saved:false,tls_mode:'disable'}]};
        else if (path.endsWith('/session')) data={token:'cache-fixture',protocol:'memcached',expires_at:new Date(Date.now()+3600000).toISOString()};
        else if (path.endsWith('/metadata')) data={nodes:[]};
        else if (path.endsWith('/execute')) {
          const command=JSON.parse(route.request().postDataJSON().statement); commands.push(command);
          if(command.operation==='stats' && failStats) return route.fulfill({json:{code:500,message:'Unavailable'}});
          data=command.operation==='stats'?{type:'rows',columns:['name','value'],rows:Object.entries({version:'1.6',curr_items:'2',uptime:'90061',bytes:'1048576',limit_maxbytes:'16777216',get_hits:'8',get_misses:'2'}).map(([name,value])=>({name,value}))}:{type:'message',message:'STORED',affected_rows:1};
          if(zeroHits && data.rows) data.rows=data.rows.map(row=>row.name.startsWith('get_')?{...row,value:'0'}:row);
        }
        await route.fulfill({json:{code:200,data}});
      });
      const page=await context.newPage(); page.on('pageerror',e=>errors.push(e.message));
      await page.goto(`${process.env.E2E_UI_URL}/e2e/webcache.html`);
      const zh=locale==='zh-CN', button=(cn,en)=>page.getByRole('button',{name:zh?cn:en,exact:true});
      await button('服务统计','Service stats').waitFor();
      await page.locator('.webdata-cache-overview').getByText('80.0%',{exact:true}).waitFor();
      assert.equal(await page.locator('.webdata-side').getByRole('button',{name:zh?'写入 Key':'Write key',exact:true}).count(),0);
      assert.equal(await page.locator('input[type=password]').count(),0);
      assert.equal(await page.locator('.webdata-object-tree').count(),0);
      await button('服务统计','Service stats').click();
      await button('执行','Run').click();
      await page.getByText('curr_items',{exact:true}).waitFor();
      await page.getByRole('textbox',{name:'Key',exact:true}).fill('demo:key');
      await button('写入 Key','Write key').click();
      const draft=await page.locator('.webdata-code-editor textarea').inputValue();
      await button('刷新概况','Refresh overview').click();
      await page.locator('.webdata-cache-overview').getByText('80.0%',{exact:true}).waitFor();
      assert.equal(await page.locator('.webdata-code-editor textarea').inputValue(),draft);
      const beforeWrite=commands.length;
      let confirmations=0;
      page.once('dialog',async dialog=>{confirmations++; assert.equal(commands.length,beforeWrite); await dialog.dismiss();});
      await button('执行','Run').click();
      assert.equal(commands.length,beforeWrite,'write must wait for confirmation');
      assert.equal(confirmations,1);
      page.once('dialog',async dialog=>{confirmations++; await dialog.accept();});
      await button('执行','Run').click();
      await page.getByText('STORED',{exact:true}).first().waitFor();
      assert.equal(commands.length,beforeWrite+1);
      assert.equal(commands.at(-1).key,'demo:key');
      failStats=true;
      await button('刷新概况','Refresh overview').click();
      await page.locator('.webdata-cache-overview [role=alert]').waitFor();
      assert.equal(await page.locator('.webdata-cache-overview').getAttribute('aria-busy'),'false');
      failStats=false;zeroHits=true;
      await button('刷新概况','Refresh overview').click();
      await page.locator('.webdata-cache-overview').getByText('1.6',{exact:true}).waitFor();
      assert.equal(await page.locator('.webdata-cache-overview dl > div').last().locator('dd').textContent(),'—');
      zeroHits=false;
      await button('刷新概况','Refresh overview').click();
      await page.locator('.webdata-cache-overview').getByText('80.0%',{exact:true}).waitFor();
      await page.waitForTimeout(3500);
      await page.screenshot({path:`/tmp/webcache-${locale}-${theme}.png`});
      await page.setViewportSize({width:390,height:844});
      assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page must not overflow');
      await page.screenshot({path:`/tmp/webcache-mobile-${locale}-${theme}.png`});
      assert.deepEqual(errors,[]);
      console.log('PASS Memcached session, stats, write confirmation, layout',locale,theme);
      await context.close();
    }
  } finally { await browser.close(); }
})().catch(error=>{console.error(error);process.exitCode=1;});
