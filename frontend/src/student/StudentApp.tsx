import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { api } from '../lib/api'
import { calcLevel, getLevelProgress } from '../lib/level'
import type {
  AdminUser,
  LeaderboardRow,
  LoginResponse,
  PublicConfig,
  PublicCourse,
  StoreBadge,
  Student,
} from '../lib/types'
import LeaderboardModal from './LeaderboardModal'
import LoginScreen from './LoginScreen'
import RegisterModal from './RegisterModal'
import ResetModal from './ResetModal'
import StoreModal from './StoreModal'
import StudentDashboard from './StudentDashboard'
import CourseSelector from './CourseSelector'
import { matchesPattern, settle } from './util'

interface Boot {
  config: PublicConfig
  session: LoginResponse | null
}

interface SessionProps {
  onAdminSessionChange: (admin: AdminUser | null) => void
}

// Portal de alumnos: la interfaz de siempre, ahora con datos del API. No pinta nada hasta
// tener la configuración; si ya había sesión abierta, entra directo como con el login.
export default function StudentApp({ onAdminSessionChange }: SessionProps) {
  const [boot, setBoot] = useState<Boot | null>(null)
  const navigate = useNavigate()

  useEffect(() => {
    let alive = true
    let retry: number | undefined
    let restoredSession: Awaited<ReturnType<typeof api.session>> | undefined
    const load = async () => {
      try {
        // Configuración y sesión se piden juntas para evitar esperas de red en serie.
        const configRequest = api.config().then(
          (value) => ({ ok: true as const, value }),
          (error: unknown) => ({ ok: false as const, error }),
        )
        const session = restoredSession ?? await api.session()
        if (!alive) return
        // Una falla de configuración no debe consumir de nuevo la animación del
        // alumno ni impedir que una sesión admin válida abra su panel.
        if (session.success) restoredSession = session
        if (session.success && session.role === 'admin') {
          onAdminSessionChange(session.admin)
          navigate('/admin/alumnos', { replace: true })
          return
        }
        const configResult = await configRequest
        if (!alive) return
        if (!configResult.ok) throw configResult.error
        const config = configResult.value
        onAdminSessionChange(null)
        if (config.error !== undefined) throw new Error(config.error)
        setBoot({ config, session: session.success ? session : null })
      } catch (e: unknown) {
        // Sin configuración no hay qué mostrar: se reintenta en unos segundos.
        console.error(e)
        if (alive) retry = window.setTimeout(load, 5000)
      }
    }
    load()
    return () => {
      alive = false
      window.clearTimeout(retry)
    }
  }, [navigate, onAdminSessionChange])

  if (!boot) return (
    <div role="status" className="flex min-h-screen w-full items-center justify-center">
      <span className="h-10 w-10 animate-spin rounded-full border-4 border-orange-200 border-t-orange-500" />
      <span className="sr-only">Cargando…</span>
    </div>
  )
  return <Portal config={boot.config} session={boot.session} onAdminSessionChange={onAdminSessionChange} />
}

function Portal({ config, session, onAdminSessionChange }: Boot & SessionProps) {
  const navigate = useNavigate()
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
    if (switchingRef.current) return
    const epoch = courseEpoch.current
    const data = await settle(api.leaderboard())
    if (epoch !== courseEpoch.current) return
    if (data.success) {
      setLeaderboardData(data.leaderboard)
      setShowLeaderboard(true)
    }
  }

  const [isLoggedIn, setIsLoggedIn] = useState(session !== null)
  const [studentId, setStudentId] = useState('')
  const [password, setPassword] = useState('')
  const [loggingIn, setLoggingIn] = useState(false)
  const [loginError, setLoginError] = useState('')
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
  const [availableCourses, setAvailableCourses] = useState<PublicCourse[]>(session?.courses ?? [])
  const [showCourseSelection, setShowCourseSelection] = useState(session?.requiresCourseSelection ?? false)
  const [canReturnToCourse, setCanReturnToCourse] = useState(false)
  const [switchingCourseId, setSwitchingCourseId] = useState<number | null>(null)
  const [courseError, setCourseError] = useState('')
  const [buying, setBuying] = useState(false)

  // Evita mandar la misma compra dos veces con un doble clic.
  const buyingRef = useRef(false)
  const loggingInRef = useRef(false)
  const switchingRef = useRef(false)
  // Las respuestas que pertenecían a otra materia o una sesión cerrada se descartan.
  const courseEpoch = useRef(0)
  const pendingLevelUp = useRef<string | null>(null)

  // El GIF de subida de nivel se quita solo a los 4 segundos.
  useEffect(() => {
    if (!showLevelUpGif) return
    const timer = window.setTimeout(() => setShowLevelUpGif(false), 4000)
    return () => window.clearTimeout(timer)
  }, [showLevelUpGif])

  const handleLogin = async () => {
    if (loggingInRef.current) return
    loggingInRef.current = true
    setLoggingIn(true)
    setLoginError('')
    const data = await settle(api.login({ studentId, password }))
    loggingInRef.current = false
    setLoggingIn(false)
    if (data.success) {
      setPassword('')
      if (data.role === 'admin') {
        onAdminSessionChange(data.admin)
        navigate('/admin/alumnos', { replace: true })
        return
      }
      courseEpoch.current += 1
      setStudentData(withLevel(data.student))
      setAvailableCourses(data.courses)
      setShowCourseSelection(data.requiresCourseSelection)
      setCanReturnToCourse(false)
      setCourseError('')
      setIsLoggedIn(true)
      if (data.requiresCourseSelection) pendingLevelUp.current = data.levelUpOccurred ? data.student.career : null
      else if (data.levelUpOccurred && levelUpGif) setShowLevelUpGif(true)
    } else {
      setLoginError(data.error)
    }
  }

  const clearCourseContext = () => {
    setBadges([])
    setCart({})
    setShowCartModal(false)
    setShowLeaderboard(false)
    setLeaderboardData([])
    setShowLevelUpGif(false)
  }

  const handleSwitchCourse = async (courseId: number) => {
    if (switchingRef.current || buyingRef.current) return
    switchingRef.current = true
    setSwitchingCourseId(courseId)
    setCourseError('')
    const epoch = ++courseEpoch.current
    clearCourseContext()
    const data = await settle(api.switchCourse(courseId))
    switchingRef.current = false
    setSwitchingCourseId(null)
    if (epoch !== courseEpoch.current) return
    if (!data.success) {
      setCourseError(data.error)
      return
    }
    setStudentData(withLevel(data.student))
    setAvailableCourses(data.courses)
    setShowCourseSelection(false)
    setCanReturnToCourse(false)
    if (levelUpGif && (data.levelUpOccurred || pendingLevelUp.current === data.student.career)) setShowLevelUpGif(true)
    pendingLevelUp.current = null
  }

  const chooseCourse = () => {
    if (switchingRef.current || buyingRef.current) return
    courseEpoch.current += 1
    clearCourseContext()
    setCourseError('')
    setCanReturnToCourse(true)
    setShowCourseSelection(true)
  }

  // Todo vuelve al estado inicial, igual que el handleLogout de antes.
  const resetState = () => {
    courseEpoch.current += 1
    pendingLevelUp.current = null
    setIsLoggedIn(false)
    setStudentId('')
    setPassword('')
    setStudentData(null)
    setAvailableCourses([])
    setShowCourseSelection(false)
    setCanReturnToCourse(false)
    setSwitchingCourseId(null)
    setCourseError('')
    setLoginError('')
    setShowLeaderboard(false)
    setLeaderboardData([])
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
  }

  const handleLogout = async () => {
    if (switchingRef.current || buyingRef.current) return
    // Si la petición falla, igual se sale en este navegador.
    await settle(api.logout())
    resetState()
  }

  const fetchBadges = async () => {
    if (switchingRef.current) return
    const epoch = courseEpoch.current
    // El servidor ya quita las insignias ocultas para el curso de la sesión.
    const data = await settle(api.badges())
    if (epoch !== courseEpoch.current) return
    if (data.success) {
      setBadges(data.badges)
      return
    }
    setBadges([])
    // La tienda pudo cerrarse mientras el alumno tenía la sesión abierta.
    setShowCartModal(false)
    alert(data.error)
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
    if (!studentData || buyingRef.current || switchingRef.current) return
    const epoch = courseEpoch.current

    // Sólo lo que sigue en la tienda; lo demás no suma al total que ve el alumno.
    const items = badges.flatMap((b) => {
      const quantity = cart[b.id]
      return quantity ? [{ badgeId: b.id, quantity, unitCost: b.cost }] : []
    })

    buyingRef.current = true
    setBuying(true)
    const data = await settle(api.purchase({ items }))
    buyingRef.current = false
    setBuying(false)
    if (epoch !== courseEpoch.current) return
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

  return (
    <>
      {isLoggedIn && studentData && showCourseSelection ? (
        <CourseSelector
          courses={availableCourses}
          studentName={studentData.name}
          busyCourseId={switchingCourseId}
          error={courseError}
          onSelect={handleSwitchCourse}
          onLogout={handleLogout}
          onBack={canReturnToCourse ? () => { setShowCourseSelection(false); setCourseError('') } : undefined}
        />
      ) : isLoggedIn && studentData ? (
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
          courses={availableCourses}
          currentCourse={availableCourses.find((c) => c.name === studentData.career) ?? { id: 0, name: studentData.career, logoUrl: '', sideImageUrl: '' }}
          courseBusy={switchingCourseId !== null || buying}
          courseError={courseError}
          onSwitchCourse={handleSwitchCourse}
          onChooseCourse={chooseCourse}
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
          loading={loggingIn}
          error={loginError}
          setShowRegister={setShowRegister}
          setShowReset={setShowReset}
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
          courses={courses}
          selectedCareer={selectedCareer}
          setSelectedCareer={setSelectedCareer}
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

    </>
  )
}
