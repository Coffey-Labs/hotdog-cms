---
title: Home
summary: A site built with cl-cms.
layout: landing
sections:
  - hero:
      eyebrow: Built with cl-cms
      heading: Your site, in plain files
      text: Pages are Markdown. Templates make them look like this. When someone reads it, nothing runs but a web server handing out files.
      buttons:
        - { text: Read the articles, href: /articles/, primary: true }
        - { text: About this site, href: /about/ }
  - cards:
      heading: What's in the box
      from: features
  - latest:
      heading: Latest articles
      collection: articles
      count: 3
  - closing:
      heading: Make it yours
      text: Change `content/index.md` and this page changes. Change `assets/styles.css` and every page does.
      button: { text: Start with About, href: /about/ }
---
