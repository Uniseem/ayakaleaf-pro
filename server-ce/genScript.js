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
