import '../styles/globals.css'
import NoSsr from "../components/NoSsr";


function MyApp({ Component, pageProps }) {
  return (
      <NoSsr>
        <Component {...pageProps} />
      </NoSsr>
  )
}

export default MyApp
