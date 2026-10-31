import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

function context() {
  const fields = {};
  const state = {
    app: {channels: [{id:'channel',name:'NBA <feed>',categoryId:'nba',categoryName:'Basketball'}]},
    sports: {leagues:[{id:'nba',name:'NBA'}],events:[{id:'espn:one',name:'Celtics at Knicks',startUnix:1800000000}]},
    adminCategorySettings: {sportsGuide:{enabled:true,groups:[],overrides:[]}}
  };
  const ctx = vm.createContext({
    state, items:x=>Array.isArray(x)?x:[], fields,
    escapeHTML:x=>String(x??'').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;'),
    icon:()=>'<svg></svg>', byId:id=>({value:fields[id]||''}),
    defaultAdminCategorySettings:()=>({}),markAdminSettingsDraft(){},renderAdminSportsSettings(){},renderAdminPage(){}
  });
  vm.runInContext(fs.readFileSync(new URL('../internal/plugin/ui/admin.js',import.meta.url),'utf8'),ctx);
  ctx.renderAdminSportsSettings = function() {};
  ctx.renderAdminPage = function() {};
  return ctx;
}
test('guide controls render schedule choices safely and expose the saved disable choice',()=>{
  const ctx=context();
  ctx.state.adminCategorySettings.sportsGuide.enabled=false;
  const html=ctx.renderAdminSportsGuide(ctx.state.adminCategorySettings);
  assert.ok(html.includes('NBA &lt;feed>'));
  assert.ok(html.includes('Celtics at Knicks'));
  assert.ok(html.includes('Do not fill this channel'));
  assert.match(html, /<input type="checkbox"[^>]*guide[^>]*>/i);
  assert.doesNotMatch(html, /<input type="checkbox"[^>]* checked/);
});
test('group hints and manual links edit one row without dropping other choices',()=>{
  const ctx=context();
  Object.assign(ctx.fields,{'admin-guide-group':'nba','admin-guide-league':'nba','admin-guide-mode':'matchup'});
  ctx.updateAdminSportsGuide('guide-add-group',{});
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.groups[0].leagueId,'nba');
  Object.assign(ctx.fields,{'admin-guide-channel':'channel','admin-guide-event':'espn:one'});
  ctx.updateAdminSportsGuide('guide-add-link',{});
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.overrides[0].eventId,'espn:one');
  ctx.fields['admin-guide-event']='__off';
  ctx.updateAdminSportsGuide('guide-add-link',{});
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.overrides.length,1);
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.overrides[0].disabled,true);
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.groups.length,1);
  ctx.updateAdminSportsGuide('guide-remove-group',{getAttribute:()=> '0'});
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.groups.length,0);
  assert.equal(ctx.state.adminCategorySettings.sportsGuide.overrides.length,1);
});
