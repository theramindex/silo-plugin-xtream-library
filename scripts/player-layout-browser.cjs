const {chromium} = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const assert = require('node:assert/strict');
const root = path.resolve(__dirname, '..');

(async () => {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    await page.setContent('<html><body><main id="view"></main></body></html>');
    await page.addStyleTag({path: path.join(root, 'internal/plugin/ui/styles.css')});
    await page.addScriptTag({path: path.join(root, 'internal/plugin/ui/player.js')});
    await page.evaluate(() => {
      window.state = {currentChannel: {id: 'espn', name: 'ESPN', categoryName: 'Sports'}, volume: 1};
      window.byId = id => document.getElementById(id);
      window.currentProgram = () => ({title: 'First Take'});
      window.isRewindableChannel = () => false;
      window.sportsFirstPlayerActive = () => false;
      window.timeLabel = () => '';
      window.escapeHTML = text => String(text || '');
      window.favoriteMap = () => ({});
      window.icon = () => '<svg viewBox="0 0 24 24" aria-hidden="true"></svg>';
      // Layout does not require provider sessions or media playback.
      for (const name of ['updateVolumeMenu', 'renderPlayerGuidePanel', 'renderPlayerSportsDrawer', 'renderPlayerMoreMenu', 'updateFullscreenButton', 'wakePlayerChrome']) window[name] = () => {};
      window.updateAudioMenu = () => {
        byId('player-audio-button').innerHTML = '<span>Default audio</span>' + icon();
      };
      renderPlayerPage();
    });
    for (const viewport of [{width: 1024, height: 600}, {width: 1280, height: 800}, {width: 1440, height: 1000}, {width: 1920, height: 1080}]) {
      await page.setViewportSize(viewport);
      const layout = await page.evaluate(() => ({
        stage: document.querySelector('.playback-stage').getBoundingClientRect().toJSON(),
        controls: Array.from(document.querySelectorAll('.player-top button,.player-bottom-actions button')).filter(el => el.getBoundingClientRect().width > 0).map(el => ({label: el.getAttribute('aria-label') || el.textContent, rect: el.getBoundingClientRect().toJSON()})),
      }));
      assert.ok(layout.stage.right <= viewport.width + 1, `player extends beyond ${viewport.width}px window: ${JSON.stringify(layout.stage)}`);
      assert.ok(layout.stage.bottom <= viewport.height + 1, 'player extends below window');
      for (const control of layout.controls) {
        assert.ok(control.rect.left >= -1 && control.rect.right <= viewport.width + 1, `${control.label} is clipped at ${viewport.width}px`);
      }
    }
    console.log('PASS desktop player and controls stay within the viewport');
  } finally {
    await browser.close();
  }
})().catch(error => {console.error(error); process.exit(1);});
