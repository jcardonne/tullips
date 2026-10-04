import { useRef, useState } from "react";
import { Tulip } from "../lib/tulip";
import { createFileRoute } from "@tanstack/react-router";
import {
  ArrowUpRight,
  ArrowRight,
  Check,
  Globe2,
  Users,
  Send,
  Sprout,
  Sparkles,
  Clock3,
  ShieldCheck,
} from "lucide-react";
export const Route = createFileRoute("/")({ component: Landing });
const stages = [
  {
    name: "Your website",
    icon: Globe2,
    title: "Start with what makes you, you.",
    description:
      "Your product has a story. Let your AI companion understand it before you introduce yourself.",
  },
  {
    name: "Your people",
    icon: Users,
    title: "A smaller list. A better fit.",
    description:
      "Shape your audience together. See why someone fits, what is missing, and where your judgment matters.",
  },
  {
    name: "Your conversation",
    icon: Send,
    title: "Personal by design. Yours to send.",
    description:
      "Build a sequence that feels like you. Choose the pace, review the words, and decide how much to automate.",
  },
];
function Landing() {
  const [stage, setStage] = useState(0);
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);
  return (
    <div className="landing">
      <a className="skip-link" href="#main-content">
        Skip to content
      </a>
      <nav aria-label="Main navigation">
        <a className="brand" href="/">
          <Tulip />
          tullips
        </a>
        <div>
          <a href="#how-it-grows">How it grows</a>
          <a href="/login">
            Log in <ArrowUpRight size={15} />
          </a>
          <a className="button primary" href="/login?signup=true">
            Start growing <ArrowRight size={16} />
          </a>
        </div>
      </nav>
      <main id="main-content">
        <section className="landing-main" aria-labelledby="hero-title">
          <span className="eyebrow hero-reveal">
            <span className="dot" /> OPEN SOURCE. HUMAN CONNECTIONS.
          </span>
          <h1 id="hero-title" className="hero-reveal">
            Good conversations.
            <br />
            <em>Great things grow.</em>
          </h1>
          <p className="hero-reveal">
            Meet the people your business was made for.
            <br />
            Thoughtful LinkedIn outreach, with AI by your side
            <br />
            and you in control.
          </p>
          <div className="actions hero-reveal">
            <a className="button primary" href="/login?signup=true">
              Plant your first seed <ArrowUpRight size={18} />
            </a>
            <a className="button secondary-link" href="/app?demo=true">
              Take a look around <ArrowRight size={16} />
            </a>
          </div>
          <div className="fine hero-reveal">
            <Check size={14} /> Free to self-host <Check size={14} /> Yours to
            shape <Check size={14} /> MIT licensed
          </div>
          <div className="garden" aria-hidden="true">
            <div className="garden-ring" />
            <div className="stem s1">
              <span />
              <i />
            </div>
            <div className="stem s2">
              <span />
              <i />
            </div>
            <div className="stem s3">
              <span />
              <i />
            </div>
            <span className="garden-note">Room for something good.</span>
            <div className="garden-card">
              <span className="icon-box">
                <Sprout size={20} />
              </span>
              <div>
                <b>A connection, not just a contact.</b>
                <p>The right people. A personal approach.</p>
              </div>
              <span className="garden-check">
                <Check size={13} />
              </span>
            </div>
          </div>
          <a className="hero-scroll" href="#how-it-grows">
            <span /> A more thoughtful way to reach out <ArrowRight size={13} />
          </a>
        </section>
        <section
          className="product-story"
          id="how-it-grows"
          aria-labelledby="story-title"
        >
          <div className="story-heading">
            <span className="eyebrow">
              FROM A LITTLE CONTEXT TO A REAL CONNECTION
            </span>
            <h2 id="story-title">
              Good growth doesn't happen
              <br />
              <em>by accident.</em>
            </h2>
            <p>
              Bring your business. Add a little intention.
              <br />
              Make the next move together.
            </p>
          </div>
          <div className="story-grid">
            <div className="story-controls">
              <div
                role="tablist"
                aria-label="Explore the outreach workflow"
                aria-orientation="vertical"
              >
                {stages.map((s, i) => (
                  <button
                    key={s.name}
                    ref={(el) => {
                      tabs.current[i] = el;
                    }}
                    id={`story-tab-${i}`}
                    role="tab"
                    aria-selected={stage === i}
                    aria-controls={`story-panel-${i}`}
                    tabIndex={stage === i ? 0 : -1}
                    className={`story-tab ${stage === i ? "active" : ""}`}
                    onClick={() => setStage(i)}
                    onKeyDown={(e) => {
                      let next = stage;
                      if (["ArrowDown", "ArrowRight"].includes(e.key))
                        next = (stage + 1) % stages.length;
                      else if (["ArrowUp", "ArrowLeft"].includes(e.key))
                        next = (stage + stages.length - 1) % stages.length;
                      else if (e.key === "Home") next = 0;
                      else if (e.key === "End") next = stages.length - 1;
                      else return;
                      e.preventDefault();
                      setStage(next);
                      tabs.current[next]?.focus();
                    }}
                  >
                    <span className="story-step">0{i + 1}</span>
                    <div>
                      <s.icon size={18} />
                      <b>{s.name}</b>
                    </div>
                    <ArrowUpRight size={16} />
                  </button>
                ))}
              </div>
              <div className="story-explainer">
                <h3>{stages[stage].title}</h3>
                <p>{stages[stage].description}</p>
              </div>
              <a className="story-cta" href="/app?demo=true">
                Explore the interactive demo <ArrowRight size={16} />
              </a>
            </div>
            <div className="story-preview">
              <div className="preview-chrome">
                <div>
                  <i />
                  <i />
                  <i />
                </div>
                <span>THE TULLIPS WAY</span>
                <span className="preview-example">Illustrative example</span>
              </div>
              {stages.map((s, i) => (
                <div
                  key={s.name}
                  id={`story-panel-${i}`}
                  role="tabpanel"
                  aria-labelledby={`story-tab-${i}`}
                  hidden={stage !== i}
                  tabIndex={0}
                  className="story-stage"
                >
                  {i === 0 ? (
                    <>
                      <div className="preview-caption">
                        <Globe2 size={15} />
                        <span>It starts with your corner of the internet.</span>
                      </div>
                      <div className="preview-url">
                        <span className="dot" /> your-company.com{" "}
                        <Check size={16} />
                      </div>
                      <div className="preview-site">
                        <span className="example-wordmark">
                          <Sprout size={18} />
                          Goodkind
                        </span>
                        <h3>
                          Less busywork.
                          <br />
                          <em>More good work.</em>
                        </h3>
                        <p>
                          A calmer way for growing teams
                          <br />
                          to plan their next big thing.
                        </p>
                        <div className="preview-mini-shapes">
                          <span />
                          <span />
                          <span />
                        </div>
                      </div>
                      <div className="preview-insight">
                        <Sparkles size={19} />
                        <div>
                          <b>Understand before reaching out.</b>
                          <p>
                            Your product, your positioning, your potential
                            customers.
                          </p>
                        </div>
                      </div>
                    </>
                  ) : i === 1 ? (
                    <>
                      <div className="preview-caption">
                        <Users size={15} />
                        <span>
                          Your audience, with the reasoning in the open.
                        </span>
                      </div>
                      <div className="audience-brief">
                        <span className="eyebrow">A PLACE TO START</span>
                        <h3>
                          Founders building
                          <br />
                          thoughtful teams.
                        </h3>
                        <div className="preview-chips">
                          <span>B2B software</span>
                          <span>11–50 people</span>
                          <span>United Kingdom</span>
                        </div>
                      </div>
                      <div className="preview-person">
                        <span className="avatar">JM</span>
                        <div>
                          <b>Jamie Morgan</b>
                          <small>Founder · Example company</small>
                        </div>
                        <span className="badge matched">Potential fit</span>
                      </div>
                      <div className="preview-evidence">
                        <ShieldCheck size={16} />
                        <p>
                          Role fits your audience. Company size needs a closer
                          look.
                          <br />
                          <b>You decide whether to continue.</b>
                        </p>
                      </div>
                    </>
                  ) : (
                    <>
                      <div className="preview-caption">
                        <Send size={15} />
                        <span>
                          Every step has a purpose. Every word is yours.
                        </span>
                      </div>
                      <div className="preview-sequence">
                        <div>
                          <span className="sequence-dot">
                            <Users size={16} />
                          </span>
                          <div>
                            <b>A simple introduction</b>
                            <p>Send a LinkedIn connection request</p>
                          </div>
                          <Check size={15} />
                        </div>
                        <div className="sequence-wait">
                          <Clock3 size={13} />2 business days · once connected
                        </div>
                        <div>
                          <span className="sequence-dot pink-dot">
                            <Send size={16} />
                          </span>
                          <div>
                            <b>Start a conversation</b>
                            <p>“Hi Jamie, I noticed what you're building…”</p>
                          </div>
                        </div>
                        <div className="sequence-wait">
                          <Clock3 size={13} />3 business days · only if no reply
                        </div>
                        <div>
                          <span className="sequence-dot">
                            <Sprout size={16} />
                          </span>
                          <div>
                            <b>A thoughtful follow-up</b>
                            <p>A little nudge, without the noise</p>
                          </div>
                        </div>
                      </div>
                      <div className="preview-control">
                        <ShieldCheck size={17} />
                        <span>You review. You choose the pace.</span>
                        <span className="badge">In your hands</span>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          </div>
        </section>
        <section className="landing-close">
          <Tulip size={46} />
          <span className="eyebrow">YOUR NEXT CHAPTER</span>
          <h2>
            There's someone out there
            <br />
            <em>you should meet.</em>
          </h2>
          <a className="button primary" href="/login?signup=true">
            Let's find your people <ArrowUpRight size={17} />
          </a>
          <p>Start small. Make it meaningful.</p>
        </section>
      </main>
      <footer>
        <a className="brand" href="/">
          <Tulip size={22} />
          tullips
        </a>
        <span>A little intention goes a long way.</span>
        <span>Open source. Open possibilities.</span>
      </footer>
    </div>
  );
}
