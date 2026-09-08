const services = require('./services')

console.log('#!/bin/bash')
console.log('set -ex')

switch (process.argv.pop()) {
  case 'install':
    console.log('yarn workspaces focus --all --production')
    break
  case 'compile':
    for (const service of services) {
      console.log('pushd', `services/${service.name}`)
      switch (service.name) {
        case 'web':
          // No browser bundle. The pages are a separate client
          // (services/frontend) with its own image, and what runs from here
          // is web's api mode, which renders nothing: webpack, the pug
          // precompile and the Pyodide download would all be built for a
          // service that was deleted. They were also most of this image's
          // build time and a good part of its size.
          //
          // The manifest is the one thing that has to exist anyway. web's
          // ExpressLocals imports it at startup whenever NODE_ENV is
          // production, before it knows whether anything will ever render a
          // page, so an empty one is the difference between the api starting
          // and crash-looping.
          console.log('mkdir -p public')
          console.log(`echo '{"entrypoints":{}}' > public/manifest.json`)
          break
        default:
          console.log(`echo ${service.name} does not require a compilation`)
      }
      console.log('popd')
    }
    break
  default:
    console.error('unknown command')
    console.log('exit 101')
    process.exit(101)
}
