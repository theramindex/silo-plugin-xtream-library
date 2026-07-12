(function(global) {
  "use strict";

  function items(value) {
    return Array.isArray(value) ? value : [];
  }

  function normalizedNameToken(value) {
    return String(value || "").trim().replace(/\s+/g, " ");
  }

  function parsedDelimitedPath(name, delimiter) {
    name = normalizedNameToken(name);
    if (!name) return [];
    const pattern = delimiter === "dash" ? /\s+-\s*/ : /\s*\|\s*/;
    return name.split(pattern).map(normalizedNameToken).filter(Boolean);
  }

  function channelNamePathParts(channel) {
    return String((channel && channel.name) || "").split("|").map(normalizedNameToken).filter(Boolean);
  }

  function appendUnduplicatedPathParts(basePath, extraParts) {
    const baseParts = String(basePath || "").split(" / ").map(normalizedNameToken).filter(Boolean);
    let additions = items(extraParts).map(normalizedNameToken).filter(Boolean);
    while (additions.length && baseParts.length && baseParts[baseParts.length - 1].toLowerCase() === additions[0].toLowerCase()) {
      additions = additions.slice(1);
    }
    return additions.length ? baseParts.concat(additions).join(" / ") : "";
  }

  function appendVirtualPathParts(basePath, extraParts, collapseDuplicates) {
    const baseParts = String(basePath || "").split(" / ").map(normalizedNameToken).filter(Boolean);
    const additions = items(extraParts).map(normalizedNameToken).filter(Boolean);
    if (!baseParts.length || !additions.length) return "";
    if (collapseDuplicates === false) return baseParts.concat(additions).join(" / ");
    return appendUnduplicatedPathParts(basePath, additions);
  }

  global.DispatcharrLineup = Object.freeze({
    appendUnduplicatedPathParts: appendUnduplicatedPathParts,
    appendVirtualPathParts: appendVirtualPathParts,
    channelNamePathParts: channelNamePathParts,
    normalizedNameToken: normalizedNameToken,
    parsedDelimitedPath: parsedDelimitedPath
  });
})(window);
