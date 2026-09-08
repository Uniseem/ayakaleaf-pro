import { createRoot } from 'react-dom/client'
import SiteSettings from '../components/site-settings'
import '../../../../../frontend/js/infrastructure/error-reporter'

const container = document.getElementById('site-settings-root')
if (container) {
  createRoot(container).render(<SiteSettings />)
}
