// The services this image installs and starts.
//
// Empty, and that is the point: every one of them is Go now, built from
// services-go and copied in as binaries. The list is kept because the build
// scripts read it, and because it is where a Node service would go back if
// one ever had to.
module.exports = []

if (require.main === module) {
  for (const service of module.exports) {
    console.log(service.name)
  }
}
