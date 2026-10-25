const puppeteer = require('puppeteer-core');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');

(async () => {
  const root = path.resolve(__dirname, '../..');
  const output = path.join(root, '.local', 'motion-preview');
  fs.mkdirSync(output, { recursive: true });
  const executablePath = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
  const browser = await puppeteer.launch({ headless: true, executablePath, userDataDir: path.join(output, 'browser-profile'), args: ['--disable-crash-reporter', '--no-first-run'] });
  try {
    const page = await browser.newPage();
    await page.goto('http://127.0.0.1:5190/');
    await page.evaluate(() => localStorage.clear());
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    for (const [name, width, height] of [['desktop', 1440, 1060], ['mobile', 390, 844], ['small-mobile', 320, 740]]) {
      await page.setViewport({ width, height, deviceScaleFactor: 1 });
      await page.goto('http://127.0.0.1:5190/', { waitUntil: 'networkidle0' });
      await page.waitForSelector('.taste-scene');
      await page.waitForFunction(() => [...document.querySelectorAll('.taste-scene img')].every(img => img.complete && img.naturalWidth));
      await new Promise(resolve => setTimeout(resolve, 600));
      const info = await page.evaluate(() => ({
        overflow: document.documentElement.scrollWidth > innerWidth,
        imageLoaded: document.querySelector('.scene-core img').naturalWidth > 0,
        titleFits: document.querySelector('.intro-copy h1').scrollWidth <= document.querySelector('.intro-copy h1').clientWidth,
        hasCEFRGuide: document.body.textContent.includes('I understand short, simple sentences'),
        brandOverlapsStatus: document.querySelector('.brand-home').getBoundingClientRect().right > document.querySelector('.provider-status').getBoundingClientRect().left,
      }));
      assert.equal(info.overflow, false, `${name}: horizontal overflow`);
      assert.equal(info.brandOverlapsStatus, false, `${name}: header overlap`);
      assert.equal(info.imageLoaded, true);
      assert.equal(info.titleFits, true, `${name}: clipped heading`);
      assert.equal(info.hasCEFRGuide, true);
      await page.screenshot({ path: path.join(output, `${name}.png`), fullPage: true });
      console.log(name, JSON.stringify(info));
    }
    await page.click('.journey-stage-buttons > button:nth-child(2)');
    assert.equal(await page.$eval('.taste-scene', scene => scene.dataset.stage), '1');
    assert.match(await page.$eval('#journey-explanation', el => el.textContent), /Discover what connects/);
    await page.click('.journey-stage-buttons > button:nth-child(3)');
    assert.equal(await page.$eval('.taste-scene', scene => scene.dataset.stage), '2');
    await page.click('.motion-control');
    assert.equal(await page.$eval('.scene-core img', img => getComputedStyle(img).animationPlayState), 'paused');
    await page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'reduce' }]);
    await page.waitForFunction(() => document.querySelector('.motion-control').disabled);
    assert.equal(await page.$eval('.scene-core img', img => getComputedStyle(img).animationName), 'none');
    const learnerArg = process.argv.indexOf('--learner-id');
    if (learnerArg !== -1) {
      const id = process.argv[learnerArg + 1];
      await page.evaluate(id => localStorage.setItem('tastelingo.learner', JSON.stringify({ id, name: 'Saved learner', favorites: [] })), id);
      await page.reload({ waitUntil: 'networkidle0' });
      await page.waitForSelector('.plan-dashboard');
      await page.setViewport({ width: 1440, height: 1060 });
      await page.screenshot({ path: path.join(output, 'learning-week.png'), fullPage: true });
      await page.setViewport({ width: 320, height: 740 });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'learning week overflow');
      await page.screenshot({ path: path.join(output, 'learning-week-mobile.png'), fullPage: true });
      console.log('Saved learning week: layout passed');
    }
    // Hold a synthetic plan request in the browser to inspect loading and cancel
    // without generating a paid plan or modifying real learner data.
    await page.evaluate(() => localStorage.clear());
    await page.setViewport({ width: 1440, height: 1060 });
    await page.emulateMediaFeatures([{ name: 'prefers-reduced-motion', value: 'no-preference' }]);
    await page.reload({ waitUntil: 'networkidle0' });
    await page.setRequestInterception(true);
    let heldRequest;
    const intercept = request => {
      const url = new URL(request.url());
      if (url.pathname === '/api/demo' && request.method() === 'POST') {
        request.respond({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'ui-motion-preview', name: 'Preview learner', favorites: [], level: 'B1', goal: 'work', daily_minutes: 30 }) });
      } else if (url.pathname === '/api/learners/ui-motion-preview/plan') {
        heldRequest = request;
      } else { request.continue(); }
    };
    page.on('request', intercept);
    await page.click('.intro-actions .btn-primary');
    await page.waitForFunction(() => document.querySelector('.plan-progress')?.textContent.includes('Building your 7-day plan'));
    await new Promise(resolve => setTimeout(resolve, 500));
    await page.screenshot({ path: path.join(output, 'loading.png'), fullPage: true });
    await page.$eval('.plan-progress .btn-ghost', button => button.click());
    await page.waitForFunction(() => !document.querySelector('.plan-progress'));
    if (heldRequest && !heldRequest.isInterceptResolutionHandled()) await heldRequest.abort().catch(() => {});
    page.off('request', intercept);
    await page.setRequestInterception(false);
    await page.evaluate(() => localStorage.clear());
    console.log('Informative loading and cancellation: passed');
    assert.deepEqual(errors, [], 'Browser exceptions');
    console.log('Interactive stages, pause, reduced motion and browser console: passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
