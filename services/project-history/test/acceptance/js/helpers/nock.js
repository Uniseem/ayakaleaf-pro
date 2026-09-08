// The tests mock history-v1, filestore and web. Which mock they get depends on
// what is being tested.
//
// In-process, nock is the right tool: it patches Node's http module and needs
// no ports. Against a service running as its own process it is the wrong one
// for exactly the same reason -- the patch does not reach the other process --
// so the mocks are served by real HTTP servers instead. See MockServices.js.
import realNock from 'nock'
import mockServices from './MockServices.js'

const external = process.env.PROJECT_HISTORY_EXTERNAL === 'true'

export default external ? mockServices : realNock
