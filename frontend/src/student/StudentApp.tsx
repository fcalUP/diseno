import { useEffect, useRef, useState } from 'react'
import { ApiError, api } from '../lib/api'
import { calcLevel, getLevelProgress } from '../lib/level'
import type {
  AdminStudent,
  LeaderboardRow,
  LoginResponse,
  PublicConfig,
  StoreBadge,
  Student,
} from '../lib/types'
import AdminDashboard from './AdminDashboard'
import AdminLoginModal from './AdminLoginModal'
import LeaderboardModal from './LeaderboardModal'
import LoginScreen from './LoginScreen'
import RegisterModal from './RegisterModal'
import ResetModal from './ResetModal'
import StoreModal from './StoreModal'
import StudentDashboard from './StudentDashboard'
import { matchesPattern, settle } from './util'

interface Boot {
  config: PublicConfig
  session: LoginResponse | null
}

// Portal de alumnos: la interfaz de siempre, ahora con datos del API. No pinta nada hasta
// tener la configuración; si ya había sesión abierta, entra directo como con el login.
export default function StudentApp() {
  const [boot, setBoot] = useState<Boot | null>(null)

  useEffect(() => {
    let alive = true
    let retry: number | undefined
    const load = () => {
      api
        .config()
        .then(async (config) => {
          if (config.error !== undefined) throw new Error(config.error)
          // /api/me registra la subida de nivel; si el efecto ya se limpió no se llama.
          if (!alive) return
          const me = await api.me().catch(() => null)
          if (alive) setBoot({ config, session: me?.success ? me : null })
        })
        .catch((e: unknown) => {
          // Sin configuración no hay qué mostrar: se reintenta en unos segundos.
          console.error(e)
          if (alive) retry = window.setTimeout(load, 5000)
        })
    }
    load()
    return () => {
      alive = false
      window.clearTimeout(retry)
    }
  }, [])

  if (!boot) return null
  return <Portal config={boot.config} session={boot.session} />
}

function Portal({ config, session }: Boot) {
  const { courses, features, levels } = config
  const firstCourse = courses[0]?.name ?? ''
  // Sin GIF configurado no hay animación que mostrar.
  const levelUpGif = features.levelUpAnimation && config.levelUpGifUrl !== ''
  const withLevel = (student: Student): Student => ({
    ...student,
    level: calcLevel(student.exp, levels),
  })

  const [showLeaderboard, setShowLeaderboard] = useState(false)
  const [leaderboardData, setLeaderboardData] = useState<LeaderboardRow[]>([])

  const fetchLeaderboard = async () => {
    const data = await settle(api.leaderboard())
    if (data.success) {
      setLeaderboardData(data.leaderboard)
      setShowLeaderboard(true)
    }
  }

  const [isLoggedIn, setIsLoggedIn] = useState(session !== null)
  const [studentId, setStudentId] = useState('')
  const [password, setPassword] = useState('')
  const [studentData, setStudentData] = useState<Student | null>(() =>
    session ? withLevel(session.student) : null,
  )
  const [badges, setBadges] = useState<StoreBadge[]>([])
  const [cart, setCart] = useState<Record<number, number>>({})
  const [showCartModal, setShowCartModal] = useState(false)
  const [showRegister, setShowRegister] = useState(false)
  const [showReset, setShowReset] = useState(false)
  const [showCodeField, setShowCodeField] = useState(false)
  const [resetId, setResetId] = useState('')
  const [resetCode, setResetCode] = useState('')
  const [resetPass, setResetPass] = useState('')
  const [showLevelUpGif, setShowLevelUpGif] = useState(
    () => levelUpGif && session?.levelUpOccurred === true,
  )
  const [selectedCareer, setSelectedCareer] = useState(firstCourse)
  const [registerCareer, setRegisterCareer] = useState(firstCourse)

  const [isAdminLoggedIn, setIsAdminLoggedIn] = useState(false)
  const [showAdminLoginModal, setShowAdminLoginModal] = useState(false)
  const [adminStudentsData, setAdminStudentsData] = useState<AdminStudent[]>([])
  const [showAdminDashboard, setShowAdminDashboard] = useState(false)

  // Evita mandar la misma compra dos veces con un doble clic.
  const buyingRef = useRef(false)

  // El GIF de subida de nivel se quita solo a los 4 segundos.
  useEffect(() => {
    if (!showLevelUpGif) return
    const timer = window.setTimeout(() => setShowLevelUpGif(false), 4000)
    return () => window.clearTimeout(timer)
  }, [showLevelUpGif])

  const handleLogin = async () => {
    const data = await settle(
      api.login({ studentId, password, career: selectedCareer }),
    )
    if (data.success) {
      setStudentData(withLevel(data.student))
      setIsLoggedIn(true)
      if (data.levelUpOccurred && levelUpGif) setShowLevelUpGif(true)
    } else {
      alert(data.error)
    }
  }

  // Todo vuelve al estado inicial, igual que el handleLogout de antes.
  const resetState = () => {
    setIsLoggedIn(false)
    setStudentId('')
    setPassword('')
    setStudentData(null)
    setBadges([])
    setCart({})
    setShowCartModal(false)
    setShowRegister(false)
    setShowReset(false)
    setShowCodeField(false)
    setResetId('')
    setResetCode('')
    setResetPass('')
    setShowLevelUpGif(false)
    setSelectedCareer(firstCourse)
    setRegisterCareer(firstCourse)
    setIsAdminLoggedIn(false)
    setShowAdminLoginModal(false)
    setAdminStudentsData([])
    setShowAdminDashboard(false)
  }

  const handleLogout = async () => {
    // Si la petición falla, igual se sale en este navegador.
    await settle(api.logout())
    resetState()
  }

  const fetchBadges = async () => {
    // El servidor ya quita las insignias ocultas para el curso de la sesión.
    const data = await settle(api.badges())
    setBadges(data.success ? data.badges : [])
  }

  const totalCartCost = badges.reduce(
    (total, b) => total + b.cost * (cart[b.id] || 0),
    0,
  )

  const addToCart = (badgeId: number, available: number) => {
    const currentQuantity = cart[badgeId] || 0
    const badge = badges.find((b) => b.id === badgeId)
    const newTotal = totalCartCost + (badge?.cost || 0)
    if (
      studentData &&
      currentQuantity < available &&
      newTotal <= studentData.monedas
    ) {
      setCart((prev) => ({ ...prev, [badgeId]: currentQuantity + 1 }))
    }
  }

  const removeFromCart = (badgeId: number) => {
    setCart((prev) => {
      const qty = prev[badgeId] || 0
      if (qty <= 1) {
        const rest = { ...prev }
        delete rest[badgeId]
        return rest
      }
      return { ...prev, [badgeId]: qty - 1 }
    })
  }

  // Una sola compra con todo el carrito; el servidor la aplica completa o nada.
  const handlePurchase = async () => {
    if (!studentData || buyingRef.current) return

    // Sólo lo que sigue en la tienda; lo demás no suma al total que ve el alumno.
    const items = badges.flatMap((b) => {
      const quantity = cart[b.id]
      return quantity ? [{ badgeId: b.id, quantity, unitCost: b.cost }] : []
    })

    buyingRef.current = true
    const data = await settle(api.purchase({ items }))
    buyingRef.current = false
    if (!data.success) {
      alert(data.error)
      return
    }

    setStudentData((prev) =>
      prev && {
        ...withLevel(prev),
        monedas: data.newCoins,
        purchases: data.purchases,
      },
    )
    setBadges((prevBadges) =>
      prevBadges.map((b) => {
        const updated = data.badges.find((u) => u.id === b.id)
        return updated ? { ...b, quantity: updated.quantity } : b
      }),
    )
    setCart({})
    setShowCartModal(false)
  }

  const closeReset = () => {
    setShowReset(false)
    setShowCodeField(false)
    setResetId('')
    setResetCode('')
    setResetPass('')
  }

  const handleSendResetCode = async () => {
    if (!resetId) return alert('Enter your ID.')
    try {
      const data = await api.sendResetCode({ id: resetId, career: selectedCareer })
      alert(data.message || data.error)
      if (data.error === undefined) setShowCodeField(true)
    } catch (err) {
      console.error(err)
      alert('Error sending code.')
    }
  }

  const handleUpdatePassword = async () => {
    if (!matchesPattern(resetPass, config.passwordPattern)) {
      alert(config.passwordError)
      return
    }
    try {
      const data = await api.resetPassword({
        id: resetId,
        code: resetCode,
        password: resetPass,
        career: selectedCareer,
      })
      alert(data.message || data.error)
      if (data.error === undefined) closeReset()
    } catch (err) {
      console.error(err)
      alert('Error updating password.')
    }
  }

  const fetchAdminStudentsData = async (career: string) => {
    try {
      const data = await api.admin.students({ career })
      setAdminStudentsData(data.students)
    } catch (error) {
      if (error instanceof ApiError) {
        alert(error.message)
      } else {
        console.error('Error fetching admin student data:', error)
        alert('Error loading student data for administrator.')
      }
      setAdminStudentsData([])
    }
  }

  const handleAdminLoginSuccess = async (career: string) => {
    setIsAdminLoggedIn(true)
    await fetchAdminStudentsData(career)
    setShowAdminDashboard(true)
    setShowAdminLoginModal(false)
  }

  // La X del panel rápido cierra la sesión de administrador y deja todo como al inicio.
  const handleAdminClose = async () => {
    try {
      await api.admin.logout()
    } catch {
      // Aunque falle la petición, el panel se cierra en este navegador.
    }
    resetState()
  }

  return (
    <>
      {isAdminLoggedIn ? null : isLoggedIn && studentData ? (
        <StudentDashboard
          studentData={studentData}
          progress={getLevelProgress(studentData.exp, studentData.level, levels)}
          features={features}
          fetchLeaderboard={fetchLeaderboard}
          openStore={() => {
            void fetchBadges()
            setShowCartModal(true)
          }}
          handleLogout={handleLogout}
        />
      ) : (
        <LoginScreen
          courses={courses}
          features={features}
          studentId={studentId}
          setStudentId={setStudentId}
          password={password}
          setPassword={setPassword}
          handleLogin={handleLogin}
          setShowRegister={setShowRegister}
          setShowReset={setShowReset}
          selectedCareer={selectedCareer}
          setSelectedCareer={setSelectedCareer}
          setShowAdminLoginModal={setShowAdminLoginModal}
        />
      )}

      {showCartModal && studentData && (
        <StoreModal
          monedas={studentData.monedas}
          badges={badges}
          cart={cart}
          totalCartCost={totalCartCost}
          addToCart={addToCart}
          removeFromCart={removeFromCart}
          handlePurchase={handlePurchase}
          onClose={() => setShowCartModal(false)}
        />
      )}

      {showLevelUpGif && (
        <div className="fixed inset-0 bg-black bg-opacity-80 flex justify-center items-center z-50">
          <img
            src={config.levelUpGifUrl}
            alt="Level Up!"
            className="w-full h-full object-contain"
          />
        </div>
      )}

      {showLeaderboard && (
        <LeaderboardModal
          leaderboardData={leaderboardData}
          onClose={() => setShowLeaderboard(false)}
        />
      )}

      {showRegister && (
        <RegisterModal
          courses={courses}
          registerCareer={registerCareer}
          setRegisterCareer={setRegisterCareer}
          passwordHint={config.passwordHintRegister}
          onClose={() => setShowRegister(false)}
        />
      )}

      {showReset && (
        <ResetModal
          resetId={resetId}
          setResetId={setResetId}
          showCodeField={showCodeField}
          resetCode={resetCode}
          setResetCode={setResetCode}
          resetPass={resetPass}
          setResetPass={setResetPass}
          passwordHint={config.passwordHintReset}
          handleSendResetCode={handleSendResetCode}
          handleUpdatePassword={handleUpdatePassword}
          onCancel={closeReset}
        />
      )}

      <AdminLoginModal
        show={showAdminLoginModal}
        onClose={() => setShowAdminLoginModal(false)}
        onAdminLoginSuccess={handleAdminLoginSuccess}
        selectedCareer={selectedCareer}
      />

      {showAdminDashboard && (
        <AdminDashboard
          onClose={handleAdminClose}
          studentsData={adminStudentsData}
          setStudentsData={setAdminStudentsData}
          selectedCareer={selectedCareer}
          levels={levels}
          onRefreshStudents={fetchAdminStudentsData}
        />
      )}
    </>
  )
}
