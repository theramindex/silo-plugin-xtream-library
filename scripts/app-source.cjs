// The browser app.js is composed from ui/*.js parts (see playerAppScriptParts
// in internal/plugin/ui_assets.go). Tests read the same composed bundle; the
// part order comes from ui_assets.go so the two cannot drift.
const fs = require('fs');
const path = require('path');

const pluginDir = path.join(__dirname, '..', 'internal', 'plugin');

function appScriptParts() {
  const go = fs.readFileSync(path.join(pluginDir, 'ui_assets.go'), 'utf8');
  const block = go.match(/var playerAppScriptParts = \[\]string\{([\s\S]*?)\}/);
  if (!block) throw new Error('playerAppScriptParts not found in ui_assets.go');
  return [...block[1].matchAll(/"(ui\/[^"]+)"/g)].map(match => match[1]);
}

function appSource() {
  return appScriptParts()
    .map(part => fs.readFileSync(path.join(pluginDir, part), 'utf8').replace(/\n+$/, ''))
    .join('\n') + '\n';
}

// topLevelFunction returns one top-level function declaration from source, so
// a test can load a unit by name whichever part file it lives in. Top-level
// declarations start at column 0 and close with "}" at column 0.
function topLevelFunction(source, name) {
  const start = source.search(new RegExp('^(async )?function ' + name + '\\(', 'm'));
  if (start < 0) throw new Error('function ' + name + ' not found');
  const end = source.indexOf('\n}', start);
  if (end < 0) throw new Error('function ' + name + ' is not closed');
  return source.slice(start, end + 2) + '\n';
}

module.exports = { appScriptParts, appSource, topLevelFunction };
