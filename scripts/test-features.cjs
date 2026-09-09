const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const assert = require('node:assert/strict');

const ui = path.join(__dirname, '../internal/plugin/ui');
const nodes = {};
const makeNode = () => ({innerHTML:'', style:{}, classList:{toggle(){}}, setAttribute(name,value){this[name]=value;}, querySelector(){return null;}, querySelectorAll(){return [];}});
const sandbox = {
  window:{location:{pathname:'/api/v1/plugins/18/xtream',search:''},addEventListener(){}},
  document:{documentElement:{dataset:{}},body:makeNode(),querySelector(){return null;},querySelectorAll(){return [];},getElementById(id){return nodes[id] || null;},addEventListener(){}},
  localStorage:{getItem(){return null;},setItem(){}},navigator:{},URLSearchParams,
  setTimeout(){return 1;},clearTimeout(){},setInterval(){return 1;},clearInterval(){},
  fetch(){return new Promise(()=>{});},console
};
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(path.join(ui,'lineup.js'),'utf8')+'\n'+fs.readFileSync(path.join(ui,'app.js'),'utf8'),sandbox);
const run = code => vm.runInContext(code,sandbox);
run(`
savePrefs = function() { return Promise.resolve(); };
showAppToast = function() {};
render = function() {};
state.app = {
  channels: [
    {id:'replay',name:'MLB REPLAY',categoryId:'replays',categoryName:'REPLAY'},
    {id:'category-replay',name:'Game 3',categoryId:'replays',categoryName:'Sports Replays'},
    {id:'news',name:'World News',categoryId:'news',categoryName:'News'},
    {id:'movie',name:'Cinema',categoryId:'movies',categoryName:'Movies'}
  ], categories:[],source:{mode:'xtream',profiles:[]},capabilities:{recordings:false},
  preferences:defaultPrefs(),
  programs:[
    {id:'future',channelId:'news',title:'World Report',startUnix:Math.floor(Date.now()/1000)+600,endUnix:Math.floor(Date.now()/1000)+1800},
    {id:'movie-guide',channelId:'movie',title:'Sports Replay Tonight',startUnix:Math.floor(Date.now()/1000),endUnix:Math.floor(Date.now()/1000)+1800}
  ],
  vod:{available:true,items:[{id:'v1',name:'World Adventure'}]},series:{available:true,items:[{id:'s1',name:'World Show'}]}
};
normalizePreferences(); rebuildProgramIndex();
state.view="mytv";
state.adminCategorySettings=defaultAdminCategorySettings();
`);
assert.equal(run('sportsEnabled()'),false);
assert.equal(run('sportsNavAvailable()'),false);
assert.equal(run('searchFilters().some(x=>x.id==="sports")'),false);
assert.equal(run('myTVSportsPeople().length'),0);
assert.equal(run('dvrEnabled()'),false);
run('state.query="unrelated guide search"; state.category="movies";');
assert.equal(run('replayChannels().map(x=>x.id).join(",")'),'replay,category-replay','only REPLAY channel names/categories qualify; guide search does not leak');
run('state.app.preferences.hiddenCategories.replays=true');
assert.equal(run('replayChannels().length'),0,'hidden categories stay hidden');
run('state.app.preferences.hiddenCategories={}; state.category=""; state.query=""; setChannelFavorite("news",true);');
assert.equal(run('favoriteMap().news'),true);
assert.match(run('myTVFavoriteChannelsHTML()'),/World News/);
run('addKeywordPass("World Report")');
assert.equal(run('myTVGuidePrograms().length'),1);
const savedPassID = run('keywordPasses()[0].id');
run('removeKeywordPass('+JSON.stringify(savedPassID)+')');
assert.equal(run('myTVGuidePrograms().length'),0);
run('state.searchType="channels"');
assert.equal(run('searchResultSections("never-a-match").length'),0,'empty channel sections are omitted');
assert.match(run('renderSearchResults("World")'),/data-save-channel/);
run('state.searchType="movies"; state.searchQuery="World"');
assert.match(run('renderSearchPageResults()'),/World Adventure/,'Xtream movies remain searchable');
run('state.searchType="shows"');
assert.match(run('renderSearchPageResults()'),/World Show/,'Xtream series remain searchable');
run('state.searchType="guide"');
assert.match(run('renderSearchPageResults()'),/World Report/);
assert.equal(run('guideSearchPrograms(state.app.channels,state.programsByChannel,"World Report",guideWindow(),20).total'),1);
run('state.adminCategorySettings.sportsEnabled=true');
assert.equal(run('sportsNavAvailable()'),true);
assert.equal(run('searchFilters().some(x=>x.id==="sports")'),true);
nodes.player = {paused:true,ended:false};
nodes['player-center-button'] = makeNode();
run('updateTimeShiftUI=function(){}; updateCenterPlayButton()');
assert.equal(nodes['player-center-button']['aria-label'],'Play');
nodes.player.paused=false;
run('state.playerWaiting=true; updateCenterPlayButton()');
assert.equal(nodes['player-center-button']['aria-label'],'Loading stream');
assert.equal(nodes['player-center-button'].disabled,true);
run('state.playerWaiting=false; updateCenterPlayButton()');
assert.equal(nodes['player-center-button']['aria-label'],'Pause');
assert.equal(nodes['player-center-button'].disabled,false);
console.log('Xtream feature behavior checks passed');
