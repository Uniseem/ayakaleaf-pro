// The services this image installs and starts.
//
// web is not one of them any more: the pages are a separate client and
// everything that was left of it -- the internal API the other services ask --
// is services-go/cmd/api now.
module.exports = [
  {
    name: 'real-time',
  },
  {
    name: 'document-updater',
  },
  {
    name: 'clsi',
  },
  {
    name: 'filestore',
  },
  {
    name: 'docstore',
  },
  {
    name: 'chat',
  },
  {
    name: 'notifications',
  },
  {
    name: 'project-history',
  },
  {
    name: 'history-v1',
  },
  {
    name: 'linked-url-proxy',
  }
]

if (require.main === module) {
  for (const service of module.exports) {
    console.log(service.name)
  }
}
