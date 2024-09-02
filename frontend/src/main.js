import { defineCustomElement } from 'vue'
import './style.css'
import Layout from './components/Layout.vue'
import Login from './components/Login.vue'

customElements.define('nonstick-layout', defineCustomElement(Layout))
customElements.define('nonstick-login', defineCustomElement(Login))

