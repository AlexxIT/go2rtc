import fs from 'fs';
import path from 'path';
import { defineConfig } from 'vitepress';

const repoRoot = path.resolve(__dirname, '..');
const srcDir = repoRoot;
const skipDirs = new Set(['.git', 'node_modules', '.vitepress', 'dist', 'scripts']);

function walkForReadmes(dir: string, results: string[]) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      if (skipDirs.has(entry.name)) {
        continue;
      }
      walkForReadmes(path.join(dir, entry.name), results);
      continue;
    }

    if (entry.isFile() && entry.name === 'README.md') {
      results.push(path.join(dir, entry.name));
    }
  }
}

function extractTitle(filePath: string) {
  const content = fs.readFileSync(filePath, 'utf8');
  const match = content.match(/^#\s+(.+)$/m);
  return match ? match[1].trim() : '';
}

function toTitleCase(value: string) {
  return value
    .replace(/[-_]/g, ' ')
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function toLink(routePath: string) {
  const clean = routePath.replace(/index\.md$/, '');
  return clean ? `/${clean}` : '/';
}

const readmeFiles: string[] = [];
walkForReadmes(srcDir, readmeFiles);

const readmePaths = readmeFiles
  .map((filePath) => path.relative(srcDir, filePath).replace(/\\/g, '/'))
  .sort();

const rewrites = Object.fromEntries(
  readmePaths.map((relPath) => [relPath, relPath.replace(/README\.md$/, 'index.md')])
);

const groupOrder = ['', 'docker', 'api', 'pkg', 'internal', 'examples', 'www'];
const groupTitles = new Map([
  ['', 'Overview'],
  ['api', 'API'],
  ['pkg', 'Packages'],
  ['internal', 'Internal'],
  ['examples', 'Examples'],
  ['docker', 'Docker'],
  ['www', 'WWW'],
]);

const groupedItems = new Map<string, Array<{ text: string; link: string }>>();

for (const relPath of readmePaths) {
  const filePath = path.join(srcDir, relPath);
  const segments = relPath.split('/');
  const groupKey = segments.length > 1 ? segments[0] : '';
  const routePath = rewrites[relPath];
  const link = toLink(routePath);
  const title = extractTitle(filePath);
  const fallback = segments.length > 1 ? segments[segments.length - 2] : 'Overview';
  const text = title || toTitleCase(fallback);

  if (!groupedItems.has(groupKey)) {
    groupedItems.set(groupKey, []);
  }
  groupedItems.get(groupKey)?.push({ text, link });
}

for (const items of groupedItems.values()) {
  items.sort((a, b) => a.text.localeCompare(b.text));
}

const orderedGroups = [...groupedItems.entries()].sort((a, b) => {
  const indexA = groupOrder.indexOf(a[0]);
  const indexB = groupOrder.indexOf(b[0]);
  if (indexA !== -1 || indexB !== -1) {
    return (indexA === -1 ? Number.POSITIVE_INFINITY : indexA) -
      (indexB === -1 ? Number.POSITIVE_INFINITY : indexB);
  }
  return a[0].localeCompare(b[0]);
});

const sidebar = orderedGroups.flatMap(([groupKey, items]) => {
  const groupTitle = groupTitles.get(groupKey) || toTitleCase(groupKey || 'Overview');
  if (items.length === 1) {
    const [item] = items;
    return [{ text: groupTitle, link: item.link }];
  }
  return [{
    text: groupTitle,
    collapsed: groupKey !== '',
    items,
  }];
});

const nav = orderedGroups
  .filter(([, items]) => items.length > 0)
  .map(([groupKey, items]) => {
    if (groupKey === '') {
      return { text: groupTitles.get(groupKey) || 'Overview', link: '/' };
    }
    const landing = items.find((item) => item.link === `/${groupKey}/`) ?? items[0];
    return {
      text: groupTitles.get(groupKey) || toTitleCase(groupKey),
      link: landing.link,
    };
  });

export default defineConfig({
  lang: 'en-US',
  title: 'go2rtc Docs',
  description: 'go2rtc documentation',
  srcDir,
  base: process.env.BASE_URL || '/',
  cleanUrls: true,
  ignoreDeadLinks: true,
  rewrites,
  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    [
      'link',
      {
        rel: 'stylesheet',
        href:
          'https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;600&family=IBM+Plex+Sans:wght@400;500;600;700&display=swap',
      },
    ],
  ],
  markdown: {
    theme: {
      light: "catppuccin-latte",
      dark: "catppuccin-mocha",
    },
  },
  themeConfig: {
    nav,
    sidebar: {
      '/': sidebar,
    },
    outline: [2, 3],
    search: {
      provider: 'local',
    },
    socialLinks: [{ icon: "github", link: "https://github.com/AlexxIT/go2rtc" }],
  },
});
